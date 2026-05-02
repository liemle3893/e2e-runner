package adapter

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// ProcessAdapter manages background process lifecycles within test suites.
type ProcessAdapter struct {
	manager *ProcessManager
}

// NewProcessAdapter constructs a ProcessAdapter with a fresh ProcessManager.
func NewProcessAdapter() *ProcessAdapter {
	return &ProcessAdapter{
		manager: NewProcessManager(),
	}
}

// Name returns the adapter's registered identifier.
func (a *ProcessAdapter) Name() string { return "process" }

// Connect is a no-op; the process adapter requires no persistent connection.
func (a *ProcessAdapter) Connect(_ context.Context) error { return nil }

// Close terminates all tracked background processes regardless of AutoTeardown.
func (a *ProcessAdapter) Close(_ context.Context) error {
	a.manager.ForceStopAll()
	return nil
}

// Health is a no-op; process execution is always available.
func (a *ProcessAdapter) Health(_ context.Context) error { return nil }

// Execute dispatches to the start or stop action handler.
func (a *ProcessAdapter) Execute(ctx context.Context, action string, params map[string]any) (*tryve.StepResult, error) {
	switch action {
	case "start":
		return a.startAction(ctx, params)
	case "stop":
		return a.stopAction(params)
	default:
		return nil, tryve.AdapterError("process", action,
			fmt.Sprintf("unsupported action %q; use \"start\" or \"stop\"", action), nil)
	}
}

// startAction launches a background process, optionally waits for readiness,
// and returns PID and port in the result data.
func (a *ProcessAdapter) startAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	command, err := getStr(params, "command")
	if err != nil {
		return nil, tryve.AdapterError("process", "start", err.Error(), err)
	}

	name := getStrDefault(params, "name", "")
	if name == "" {
		return nil, tryve.AdapterError("process", "start",
			"step name is required for process/start", nil)
	}

	// Allocate a free port.
	port, err := allocateFreePort()
	if err != nil {
		return nil, tryve.AdapterError("process", "start",
			fmt.Sprintf("failed to allocate free port: %v", err), err)
	}
	portStr := strconv.Itoa(port)

	// Replace {{free_port}} and ${free_port} tokens in command, env, and readiness.
	command = replaceFreePort(command, portStr)
	envMap := getMap(params, "env")
	if envMap != nil {
		envMap = replaceFreePortInMap(envMap, portStr)
	}
	readinessMap := getMap(params, "readiness")
	if readinessMap != nil {
		readinessMap = replaceFreePortInMap(readinessMap, portStr)
	}

	autoTeardown := true
	if v, ok := params["auto_teardown"]; ok {
		if b, ok := v.(bool); ok {
			autoTeardown = b
		}
	}

	teardownTimeout := 5 * time.Second
	if v, ok := params["teardown_timeout"].(string); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			teardownTimeout = d
		}
	}

	cwd := getStrDefault(params, "cwd", "")

	opts := StartOpts{
		Name:            name,
		Command:         command,
		Env:             envMap,
		Cwd:             cwd,
		Port:            port,
		AutoTeardown:    autoTeardown,
		TeardownTimeout: teardownTimeout,
	}

	var proc *ManagedProcess
	duration, startErr := MeasureDuration(func() error {
		var e error
		proc, e = a.manager.Start(ctx, opts)
		if e != nil {
			return e
		}

		// Readiness probe.
		if readinessMap != nil {
			cfg := parseReadinessConfig(readinessMap)
			if e = WaitForReady(ctx, cfg, proc.Done()); e != nil {
				_ = a.manager.Stop(name, syscall.SIGKILL, 2*time.Second)
				stderr := proc.Stderr()
				stdout := proc.Stdout()
				msg := fmt.Sprintf("readiness probe failed for %q: %v", name, e)
				if stderr != "" {
					msg += "\nstderr: " + truncate(stderr, 500)
				}
				if stdout != "" {
					msg += "\nstdout: " + truncate(stdout, 500)
				}
				return fmt.Errorf("%s", msg)
			}
		}
		return nil
	})
	if startErr != nil {
		return nil, tryve.AdapterError("process", "start", startErr.Error(), startErr)
	}

	data := map[string]any{
		"pid":  float64(proc.PID),
		"port": float64(port),
	}
	return SuccessResult(data, duration, nil), nil
}

// stopAction terminates a background process by name or PID.
func (a *ProcessAdapter) stopAction(params map[string]any) (*tryve.StepResult, error) {
	sig := parseSignal(getStrDefault(params, "signal", "SIGTERM"))
	timeout := 5 * time.Second
	if v, ok := params["timeout"].(string); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}

	target := getStrDefault(params, "target", "")
	if target != "" {
		duration, stopErr := MeasureDuration(func() error {
			return a.manager.Stop(target, sig, timeout)
		})
		if stopErr != nil {
			return nil, tryve.AdapterError("process", "stop", stopErr.Error(), stopErr)
		}
		data := map[string]any{"stopped": target}
		return SuccessResult(data, duration, nil), nil
	}

	// Fall back to PID-based stop.
	pidVal, ok := params["pid"]
	if !ok {
		return nil, tryve.AdapterError("process", "stop",
			"either 'target' (process name) or 'pid' is required", nil)
	}
	pid, err := toIntFromAny(pidVal)
	if err != nil {
		return nil, tryve.AdapterError("process", "stop",
			fmt.Sprintf("invalid pid value: %v", err), err)
	}

	duration, stopErr := MeasureDuration(func() error {
		return a.manager.StopByPID(pid, sig, timeout)
	})
	if stopErr != nil {
		return nil, tryve.AdapterError("process", "stop", stopErr.Error(), stopErr)
	}
	data := map[string]any{"stopped": float64(pid)}
	return SuccessResult(data, duration, nil), nil
}

// allocateFreePort binds to a random port and returns it.
func allocateFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// replaceFreePort replaces {{free_port}} and ${free_port} tokens in a string.
func replaceFreePort(s, port string) string {
	s = strings.ReplaceAll(s, "{{free_port}}", port)
	s = strings.ReplaceAll(s, "${free_port}", port)
	return s
}

// replaceFreePortInMap replaces free_port tokens in all string values of a map.
func replaceFreePortInMap(m map[string]any, port string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = replaceFreePort(s, port)
		} else {
			out[k] = v
		}
	}
	return out
}

// parseReadinessConfig converts a raw YAML map into a ReadinessConfig.
func parseReadinessConfig(m map[string]any) ReadinessConfig {
	cfg := ReadinessConfig{}
	if v, ok := m["http"].(string); ok {
		cfg.HTTP = v
	}
	if v, ok := m["tcp"].(string); ok {
		cfg.TCP = v
	}
	if v, ok := m["cmd"].(string); ok {
		cfg.Cmd = v
	}
	if v, ok := m["timeout"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Timeout = d
		}
	}
	if v, ok := m["interval"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Interval = d
		}
	}
	return cfg
}

// parseSignal converts a signal name string to a syscall.Signal.
func parseSignal(s string) syscall.Signal {
	switch strings.ToUpper(s) {
	case "SIGKILL", "KILL":
		return syscall.SIGKILL
	case "SIGINT", "INT":
		return syscall.SIGINT
	default:
		return syscall.SIGTERM
	}
}

// toIntFromAny coerces common numeric types and strings to int.
func toIntFromAny(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case string:
		return strconv.Atoi(n)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

// truncate shortens a string to at most n bytes, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
