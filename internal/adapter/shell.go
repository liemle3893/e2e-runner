package adapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// defaultShellTimeout bounds any command that names no timeout of its own, so a
// process that never returns cannot hold a run open indefinitely.
const defaultShellTimeout = 60000

// ShellConfig holds configuration options for the ShellAdapter.
type ShellConfig struct {
	// DefaultTimeout is the default command timeout in milliseconds
	// (0 = defaultShellTimeout).
	DefaultTimeout int
	// DefaultCwd is the default working directory for commands (empty = inherit).
	DefaultCwd string
	// Allow lists regular expressions a command must match to run. When empty,
	// every command is permitted; when non-empty the adapter is deny-by-default
	// and anything unmatched is refused.
	Allow []*regexp.Regexp
	// Deny lists regular expressions that refuse a command outright. Deny is
	// evaluated before Allow.
	Deny []*regexp.Regexp
	// EnvAllow names the environment variables a command inherits. When empty the
	// full process environment is passed through, which hands every command in
	// the suite whatever credentials the shell happens to be holding.
	EnvAllow []string
	// policySource records where the policy came from, for error messages.
	policySource string
	// Compat selects whether the default timeout fallback applies.
	Compat tryve.CompatMode
}

// NewShellAdapterFromConfig builds a ShellAdapter from an environment's "shell"
// configuration block.
//
// Recognised keys:
//   - "defaultTimeout" (int, ms)   — bound applied to commands that set none.
//   - "cwd"            (string)    — working directory, relative to configDir.
//   - "allow"          ([]string)  — regexes; when present, deny-by-default.
//   - "deny"           ([]string)  — regexes refused before allow is consulted.
//   - "env"            ([]string)  — environment variables commands may inherit.
func NewShellAdapterFromConfig(cfg map[string]any, configDir string, mode tryve.CompatMode) *ShellAdapter {
	sc := &ShellConfig{policySource: "e2e.config.yaml", Compat: mode}

	if cfg == nil {
		return NewShellAdapter(sc)
	}

	switch v := cfg["defaultTimeout"].(type) {
	case int:
		sc.DefaultTimeout = v
	case float64:
		sc.DefaultTimeout = int(v)
	}

	if cwd, ok := cfg["cwd"].(string); ok && cwd != "" {
		if filepath.IsAbs(cwd) {
			sc.DefaultCwd = cwd
		} else {
			sc.DefaultCwd = filepath.Join(configDir, cwd)
		}
	}

	sc.Allow = compilePatterns(cfg["allow"])
	sc.Deny = compilePatterns(cfg["deny"])
	sc.EnvAllow = stringList(cfg["env"])

	return NewShellAdapter(sc)
}

// compilePatterns turns a YAML list of regex strings into compiled patterns.
// A pattern that does not compile is skipped rather than aborting the run; the
// remaining rules still apply.
func compilePatterns(v any) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, raw := range stringList(v) {
		re, err := regexp.Compile(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN  shell policy: ignoring invalid pattern %q: %v\n", raw, err)
			continue
		}
		out = append(out, re)
	}
	return out
}

// stringList coerces a YAML scalar or sequence into a slice of strings.
func stringList(v any) []string {
	switch typed := v.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return typed
	}
	return nil
}

// ShellAdapter executes shell commands via os/exec.
// Connect and Close are no-ops because shell execution requires no persistent
// connection state.
type ShellAdapter struct {
	config *ShellConfig
}

// NewShellAdapter constructs a ShellAdapter with the given configuration.
// config must not be nil.
func NewShellAdapter(config *ShellConfig) *ShellAdapter {
	if config == nil {
		config = &ShellConfig{}
	}
	return &ShellAdapter{config: config}
}

// Name returns the adapter's registered identifier.
func (a *ShellAdapter) Name() string { return "shell" }

// Connect is a no-op for the shell adapter; shell execution requires no
// persistent connection.
func (a *ShellAdapter) Connect(_ context.Context) error { return nil }

// Close is a no-op for the shell adapter.
func (a *ShellAdapter) Close(_ context.Context) error { return nil }

// Health is a no-op for the shell adapter; shell execution is always available
// when the OS is reachable.
func (a *ShellAdapter) Health(_ context.Context) error { return nil }

// Execute runs the named action with the provided parameters.
// Only the "exec" action is supported.
func (a *ShellAdapter) Execute(ctx context.Context, action string, params map[string]any) (*tryve.StepResult, error) {
	if action != "exec" {
		return nil, tryve.AdapterError(
			"shell",
			action,
			fmt.Sprintf("unsupported action %q; only \"exec\" is supported", action),
			nil,
		)
	}
	return a.execAction(ctx, params)
}

// execAction implements the "exec" action: run a shell command and collect its
// stdout, stderr, and exit code.
func (a *ShellAdapter) execAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	command, err := getStr(params, "command")
	if err != nil {
		return nil, tryve.AdapterError("shell", "exec", err.Error(), err)
	}

	if err := a.checkPolicy(command); err != nil {
		return nil, err
	}

	cwd := getStrDefault(params, "cwd", a.config.DefaultCwd)
	extraEnv := getMap(params, "env")

	// Bound every command. Without a deadline a hung process holds the whole run
	// until the enclosing test timeout, with no indication of which step stalled.
	// The shell adapter applied no timeout at all before the adapters area
	// changed: a step's `timeout`, the configured `defaultTimeout`, and the
	// built-in fallback all arrived together, so all three follow that area.
	timeoutMs := 0
	if tryve.CompatOrDefault(ctx, a.config.Compat).Modern(tryve.CompatAdapters) {
		timeoutMs = getIntDefault(params, "timeout", a.config.DefaultTimeout)
		if timeoutMs <= 0 {
			timeoutMs = defaultShellTimeout
		}
	}

	runCtx := ctx
	if timeoutMs > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}

	cmd := buildCommand(runCtx, command)

	if cwd != "" {
		cmd.Dir = cwd
	}

	cmd.Env = a.buildEnv(extraEnv)

	// Run the command in its own process group so that killing it on timeout
	// also kills the children it spawned; `sh -c` otherwise leaves them running.
	setProcessGroup(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	var duration, runErr = MeasureDuration(func() error {
		if startErr := cmd.Start(); startErr != nil {
			return startErr
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		case waitErr := <-done:
			return waitErr
		case <-runCtx.Done():
			killProcessGroup(cmd)
			<-done
			return runCtx.Err()
		}
	})

	exitCode := 0
	if runErr != nil {
		switch {
		case errors.Is(runErr, context.DeadlineExceeded) && ctx.Err() == nil:
			return nil, tryve.AdapterError("shell", "exec",
				fmt.Sprintf("command exceeded its timeout of %dms: %s", timeoutMs, truncate(command, 120)), runErr)
		case errors.As(runErr, new(*exec.ExitError)):
			var exitErr *exec.ExitError
			errors.As(runErr, &exitErr)
			exitCode = exitErr.ExitCode()
		default:
			// Actual execution failure (e.g. command not found).
			return nil, tryve.AdapterError("shell", "exec", runErr.Error(), runErr)
		}
	}

	data := map[string]any{
		"stdout":   strings.TrimRight(stdout.String(), "\r\n"),
		"stderr":   strings.TrimRight(stderr.String(), "\r\n"),
		"exitCode": float64(exitCode),
	}

	// Non-zero exit is NOT an adapter error. The exitCode is in the data
	// and can be checked via assertions (assert: exitCode: 0).
	// This matches the TS e2e-runner behavior.
	return SuccessResult(data, duration, nil), nil
}

// buildCommand constructs the exec.Cmd appropriate for the current OS.
// On Windows it uses "cmd /C"; on all other platforms it uses "sh -c".
func buildCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}

// checkPolicy applies the configured command policy, refusing anything denied or
// — when an allow list is configured — anything not explicitly permitted.
func (a *ShellAdapter) checkPolicy(command string) error {
	for _, deny := range a.config.Deny {
		if deny.MatchString(command) {
			return tryve.AdapterError("shell", "exec", fmt.Sprintf(
				"command refused by the shell 'deny' policy in %s (matched %q): %s",
				a.config.policySource, deny.String(), truncate(command, 200)), nil)
		}
	}

	if len(a.config.Allow) == 0 {
		return nil
	}
	for _, allow := range a.config.Allow {
		if allow.MatchString(command) {
			return nil
		}
	}
	return tryve.AdapterError("shell", "exec", fmt.Sprintf(
		"command is not permitted by the shell 'allow' policy in %s: %s\n"+
			"Add a pattern that matches it under adapters.shell.allow, or remove the allow list to permit any command.",
		a.config.policySource, truncate(command, 200)), nil)
}

// buildEnv assembles the environment a command runs with.
//
// With an allow list configured, only the named variables are inherited. Without
// one the full process environment is passed through, which means every command
// in the suite sees whatever credentials the developer's shell is holding.
// Extra variables supplied by the step always take precedence.
func (a *ShellAdapter) buildEnv(envMap map[string]any) []string {
	var base []string
	if len(a.config.EnvAllow) > 0 {
		base = make([]string, 0, len(a.config.EnvAllow))
		for _, name := range a.config.EnvAllow {
			if val, ok := os.LookupEnv(name); ok {
				base = append(base, fmt.Sprintf("%s=%s", name, val))
			}
		}
	} else {
		base = os.Environ()
	}

	if len(envMap) == 0 {
		return base
	}

	env := make([]string, len(base), len(base)+len(envMap))
	copy(env, base)
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%v", k, v))
	}
	return env
}

// truncate shortens s for inclusion in an error message.
func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

// getStr retrieves a required string parameter from params.
// Returns an error if the key is absent or its value is not a string.
func getStr(params map[string]any, key string) (string, error) {
	v, ok := params[key]
	if !ok {
		return "", fmt.Errorf("required parameter %q is missing", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("parameter %q must be a string, got %T", key, v)
	}
	return s, nil
}

// getStrDefault retrieves an optional string parameter from params, returning
// defaultVal when the key is absent or its value is not a string.
func getStrDefault(params map[string]any, key, defaultVal string) string {
	v, ok := params[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}

// getMap retrieves an optional map[string]any parameter from params.
// Returns nil when the key is absent or the value is not a map[string]any.
func getMap(params map[string]any, key string) map[string]any {
	v, ok := params[key]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return m
}
