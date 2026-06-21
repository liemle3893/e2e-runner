package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// maxDiagnosticBytes is the cap on stdout/stderr buffers kept for diagnostics.
const maxDiagnosticBytes = 4096

// ManagedProcess represents a running background process tracked by ProcessManager.
type ManagedProcess struct {
	Name            string
	Cmd             *exec.Cmd
	PID             int
	Port            int
	AutoTeardown    bool
	TeardownTimeout time.Duration
	stdout          limitedBuffer
	stderr          limitedBuffer
	done            chan struct{}
	exitErr         error
}

// Done returns a channel that is closed when the process exits.
func (p *ManagedProcess) Done() <-chan struct{} { return p.done }

// Stdout returns the tail of captured stdout (up to maxDiagnosticBytes).
func (p *ManagedProcess) Stdout() string { return p.stdout.String() }

// Stderr returns the tail of captured stderr (up to maxDiagnosticBytes).
func (p *ManagedProcess) Stderr() string { return p.stderr.String() }

// ExitErr returns the error from the process exit, if any.
func (p *ManagedProcess) ExitErr() error { return p.exitErr }

// StartOpts configures a background process to be started by ProcessManager.
type StartOpts struct {
	Name            string
	Command         string
	Env             map[string]any
	Cwd             string
	Port            int
	AutoTeardown    bool
	TeardownTimeout time.Duration
}

// ProcessManager tracks running background processes and provides lifecycle management.
type ProcessManager struct {
	mu        sync.Mutex
	processes map[string]*ManagedProcess
}

// NewProcessManager returns an empty, ready-to-use ProcessManager.
func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*ManagedProcess),
	}
}

// Start launches a background process with the given options and tracks it by name.
func (pm *ProcessManager) Start(ctx context.Context, opts StartOpts) (*ManagedProcess, error) {
	pm.mu.Lock()
	if _, exists := pm.processes[opts.Name]; exists {
		pm.mu.Unlock()
		return nil, fmt.Errorf("process %q is already running", opts.Name)
	}
	pm.mu.Unlock()

	cmd := buildCommand(ctx, opts.Command)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	cmd.Env = buildEnv(opts.Env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	proc := &ManagedProcess{
		Name:            opts.Name,
		Cmd:             cmd,
		Port:            opts.Port,
		AutoTeardown:    opts.AutoTeardown,
		TeardownTimeout: opts.TeardownTimeout,
		stdout:          limitedBuffer{max: maxDiagnosticBytes},
		stderr:          limitedBuffer{max: maxDiagnosticBytes},
		done:            make(chan struct{}),
	}

	cmd.Stdout = &proc.stdout
	cmd.Stderr = &proc.stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process %q: %w", opts.Name, err)
	}

	proc.PID = cmd.Process.Pid

	go func() {
		proc.exitErr = cmd.Wait()
		close(proc.done)
	}()

	pm.mu.Lock()
	pm.processes[opts.Name] = proc
	pm.mu.Unlock()

	return proc, nil
}

// Stop terminates a process by name with the given signal and timeout.
func (pm *ProcessManager) Stop(name string, sig syscall.Signal, timeout time.Duration) error {
	pm.mu.Lock()
	proc, ok := pm.processes[name]
	if !ok {
		pm.mu.Unlock()
		return fmt.Errorf("no process named %q is running", name)
	}
	pm.mu.Unlock()

	err := killProcess(proc, sig, timeout)

	pm.mu.Lock()
	delete(pm.processes, name)
	pm.mu.Unlock()

	return err
}

// StopByPID terminates a process by raw PID with the given signal and timeout.
func (pm *ProcessManager) StopByPID(pid int, sig syscall.Signal, timeout time.Duration) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process with PID %d not found: %w", pid, err)
	}
	if err := p.Signal(sig); err != nil {
		return fmt.Errorf("failed to signal PID %d: %w", pid, err)
	}

	done := make(chan struct{})
	go func() {
		_, _ = p.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		_ = p.Kill()
		return nil
	}
}

// StopAll terminates all tracked processes that have AutoTeardown set, using each
// process's individual TeardownTimeout.
func (pm *ProcessManager) StopAll() {
	pm.stopProcesses(false)
}

// ForceStopAll terminates all tracked processes regardless of AutoTeardown.
func (pm *ProcessManager) ForceStopAll() {
	pm.stopProcesses(true)
}

// stopProcesses terminates tracked processes. When forceAll is true, all processes
// are stopped; otherwise only those with AutoTeardown set.
func (pm *ProcessManager) stopProcesses(forceAll bool) {
	pm.mu.Lock()
	names := make([]string, 0, len(pm.processes))
	for name, proc := range pm.processes {
		if forceAll || proc.AutoTeardown {
			names = append(names, name)
		}
	}
	pm.mu.Unlock()

	for _, name := range names {
		pm.mu.Lock()
		proc, ok := pm.processes[name]
		pm.mu.Unlock()
		if !ok {
			continue
		}
		timeout := proc.TeardownTimeout
		if timeout == 0 {
			timeout = 5 * time.Second
		}
		_ = killProcess(proc, syscall.SIGTERM, timeout)
		pm.mu.Lock()
		delete(pm.processes, name)
		pm.mu.Unlock()
	}
}

// Get returns the managed process with the given name, if it exists.
func (pm *ProcessManager) Get(name string) (*ManagedProcess, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	p, ok := pm.processes[name]
	return p, ok
}

// killProcess sends sig to the process group, waits up to timeout, then sends SIGKILL.
func killProcess(proc *ManagedProcess, sig syscall.Signal, timeout time.Duration) error {
	select {
	case <-proc.done:
		return nil
	default:
	}

	// Send signal to the entire process group (-PID).
	if err := syscall.Kill(-proc.PID, sig); err != nil {
		// Process may have already exited.
		select {
		case <-proc.done:
			return nil
		default:
			return fmt.Errorf("failed to signal process %q (PID %d): %w", proc.Name, proc.PID, err)
		}
	}

	select {
	case <-proc.done:
		return nil
	case <-time.After(timeout):
		_ = syscall.Kill(-proc.PID, syscall.SIGKILL)
		select {
		case <-proc.done:
			return nil
		case <-time.After(5 * time.Second):
			return fmt.Errorf("process %q (PID %d) did not exit after SIGKILL", proc.Name, proc.PID)
		}
	}
}

// limitedBuffer is a bytes.Buffer that keeps only the last max bytes.
type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

// Write appends data, trimming the front if the buffer exceeds max.
func (lb *limitedBuffer) Write(p []byte) (int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	n, err := lb.buf.Write(p)
	if lb.buf.Len() > lb.max {
		excess := lb.buf.Len() - lb.max
		lb.buf.Next(excess)
	}
	return n, err
}

// String returns the buffer contents.
func (lb *limitedBuffer) String() string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.buf.String()
}
