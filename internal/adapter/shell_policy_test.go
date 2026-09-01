package adapter_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// TestShellAllowList checks that configuring an allow list makes the adapter
// deny-by-default, so a test file cannot run arbitrary commands on the machine.
func TestShellAllowList(t *testing.T) {
	a := adapter.NewShellAdapterFromConfig(map[string]any{
		"allow": []any{`^echo `, `^node scripts/e2e/`},
	}, ".", tryve.ModernCompat())

	if _, err := a.Execute(context.Background(), "exec", map[string]any{
		"command": "echo permitted",
	}); err != nil {
		t.Errorf("an allowed command should run: %v", err)
	}

	_, err := a.Execute(context.Background(), "exec", map[string]any{
		"command": "curl https://example.com",
	})
	if err == nil {
		t.Fatalf("a command outside the allow list must be refused")
	}
	if !strings.Contains(err.Error(), "allow") {
		t.Errorf("the refusal should name the policy that blocked it, got: %v", err)
	}
}

// TestShellDenyListBeatsAllow checks that deny is evaluated first.
func TestShellDenyListBeatsAllow(t *testing.T) {
	a := adapter.NewShellAdapterFromConfig(map[string]any{
		"allow": []any{`.*`},
		"deny":  []any{`rm -rf`},
	}, ".", tryve.ModernCompat())

	_, err := a.Execute(context.Background(), "exec", map[string]any{
		"command": "rm -rf /tmp/nothing-here",
	})
	if err == nil {
		t.Fatalf("a denied command must be refused even when the allow list matches")
	}
	if !strings.Contains(err.Error(), "deny") {
		t.Errorf("the refusal should name the deny policy, got: %v", err)
	}
}

// TestShellNoPolicyAllowsEverything preserves the behaviour of existing suites
// that have no shell policy configured.
func TestShellNoPolicyAllowsEverything(t *testing.T) {
	a := adapter.NewShellAdapterFromConfig(nil, ".", tryve.ModernCompat())

	if _, err := a.Execute(context.Background(), "exec", map[string]any{
		"command": "echo anything",
	}); err != nil {
		t.Errorf("without a policy any command should run: %v", err)
	}
}

// TestShellEnvAllowList checks that commands inherit only the named variables,
// so a suite's shell steps are not handed every credential in the developer's
// environment.
func TestShellEnvAllowList(t *testing.T) {
	t.Setenv("TRYVE_POLICY_KEEP", "kept")
	t.Setenv("TRYVE_POLICY_SECRET", "secret-value")

	a := adapter.NewShellAdapterFromConfig(map[string]any{
		"env": []any{"PATH", "TRYVE_POLICY_KEEP"},
	}, ".", tryve.ModernCompat())

	res, err := a.Execute(context.Background(), "exec", map[string]any{
		"command": "echo \"$TRYVE_POLICY_KEEP/$TRYVE_POLICY_SECRET\"",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stdout, _ := res.Data["stdout"].(string)
	if stdout != "kept/" {
		t.Errorf("expected only the allowed variable to be visible, got %q", stdout)
	}
}

// TestShellTimeoutIsEnforced covers the per-step timeout, which the adapter
// previously ignored entirely: a step declaring `timeout: 500` ran to completion
// however long it took.
func TestShellTimeoutIsEnforced(t *testing.T) {
	a := adapter.NewShellAdapterFromConfig(nil, ".", tryve.ModernCompat())

	start := time.Now()
	_, err := a.Execute(context.Background(), "exec", map[string]any{
		"command": "sleep 5",
		"timeout": 300,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("a command that outlives its timeout must fail")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("the failure should name the timeout, got: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("the command should have been killed promptly, took %s", elapsed)
	}
}

// TestShellTimeoutKillsChildren checks that the whole process group is signalled,
// not just the shell: a backgrounded process started by the command must not
// outlive the step that spawned it.
func TestShellTimeoutKillsChildren(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no POSIX shell available")
	}

	marker := t.TempDir() + "/child-was-still-running"
	a := adapter.NewShellAdapterFromConfig(nil, ".", tryve.ModernCompat())

	_, err := a.Execute(context.Background(), "exec", map[string]any{
		"command": "(sleep 2; touch " + marker + ") & sleep 5",
		"timeout": 300,
	})
	if err == nil {
		t.Fatalf("expected the step to time out")
	}

	// Wait past the point where the orphaned child would have created the file.
	time.Sleep(2500 * time.Millisecond)
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Errorf("a backgrounded child survived its step's timeout")
	}
}

// TestShellDefaultTimeoutFromConfig checks that the configured default applies
// to steps that name no timeout of their own.
func TestShellDefaultTimeoutFromConfig(t *testing.T) {
	a := adapter.NewShellAdapterFromConfig(map[string]any{
		"defaultTimeout": 300,
	}, ".", tryve.ModernCompat())

	_, err := a.Execute(context.Background(), "exec", map[string]any{"command": "sleep 5"})
	if err == nil {
		t.Errorf("the configured default timeout should bound a command that sets none")
	}
}

// TestLegacyShellHasNoDefaultTimeout pins the pre-v2 behaviour: a command with no
// timeout of its own ran unbounded. Only an explicit timeout applied.
func TestLegacyShellHasNoDefaultTimeout(t *testing.T) {
	a := adapter.NewShellAdapterFromConfig(nil, ".", tryve.LegacyCompat())

	start := time.Now()
	res, err := a.Execute(context.Background(), "exec", map[string]any{"command": "sleep 1"})
	if err != nil {
		t.Fatalf("legacy should not bound an untimed command: %v", err)
	}
	if code, _ := res.Data["exitCode"].(float64); code != 0 {
		t.Errorf("expected the command to complete, got exit %v", code)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Errorf("the command should have run to completion")
	}

	// Even an explicit timeout is ignored: the adapter had no timeout support at
	// all, so honouring one here would still change what an existing step does.
	start = time.Now()
	if _, err := a.Execute(context.Background(), "exec", map[string]any{
		"command": "sleep 1", "timeout": 100,
	}); err != nil {
		t.Errorf("legacy ignores a step timeout entirely: %v", err)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Errorf("the command should have run to completion under legacy compatibility")
	}
}
