package doctor

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/liemle3893/autoflow/internal/autoflow/config"
)

// execLookPath is exposed as a var so tests can stub lookup for sbx/docker.
var execLookPath = exec.LookPath

// runCombined is a var-wrapped runner used by sandbox checks so tests can
// intercept without shelling out. It returns (combinedOutput, err).
var runCombined = func(ctx context.Context, name string, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// checkCodingAgent validates .autoflow/config.json has a recognised coding_agent.
func checkCodingAgent(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "coding_agent"}
		c, err := config.Read(root)
		if err != nil {
			r.Status = Fail
			r.Detail = err.Error()
			return r
		}
		switch c.CodingAgent {
		case "claude", "copilot":
			r.Status = OK
			r.Detail = c.CodingAgent
		case "":
			r.Status = Fail
			r.Detail = "unset — run `autoflow deliver config set coding_agent claude|copilot`"
		default:
			r.Status = Fail
			r.Detail = "unknown agent " + c.CodingAgent + " (expected claude|copilot)"
		}
		return r
	}
}

// sandboxEnabled returns (enabled, name) from the on-disk config.
func sandboxEnabled(root string) (bool, string) {
	c, err := config.Read(root)
	if err != nil || c == nil {
		return false, ""
	}
	name := c.Sandbox.Name
	if name == "" {
		name = filepath.Base(root)
	}
	return c.Sandbox.Enabled, name
}

// checkSbxInstalled verifies `sbx` is on PATH. Skipped when sandbox.enabled=false.
func checkSbxInstalled(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "sbx installed"}
		enabled, _ := sandboxEnabled(root)
		if !enabled {
			r.Status = Skip
			r.Detail = "sandbox.enabled=false"
			return r
		}
		if _, err := execLookPath("sbx"); err != nil {
			r.Status = Fail
			r.Detail = "sbx not found — run `brew install docker/tap/sbx`"
			return r
		}
		r.Status = OK
		return r
	}
}

// checkSbxVersion runs `sbx version` and reports the string.
func checkSbxVersion(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "sbx version"}
		enabled, _ := sandboxEnabled(root)
		if !enabled {
			r.Status = Skip
			r.Detail = "sandbox.enabled=false"
			return r
		}
		if _, err := execLookPath("sbx"); err != nil {
			r.Status = Skip
			r.Detail = "sbx not installed"
			return r
		}
		out, err := runCombined(ctx, "sbx", "version")
		if err != nil {
			r.Status = Fail
			r.Detail = "sbx version failed: " + strings.TrimSpace(out)
			return r
		}
		r.Status = OK
		r.Detail = strings.TrimSpace(out)
		return r
	}
}

// checkSbxDocker runs `docker info` so we know the daemon is up.
func checkSbxDocker(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "docker running"}
		enabled, _ := sandboxEnabled(root)
		if !enabled {
			r.Status = Skip
			r.Detail = "sandbox.enabled=false"
			return r
		}
		if _, err := execLookPath("docker"); err != nil {
			r.Status = Fail
			r.Detail = "docker not installed"
			return r
		}
		if _, err := runCombined(ctx, "docker", "info"); err != nil {
			r.Status = Fail
			r.Detail = "docker info failed — start Docker Desktop"
			return r
		}
		r.Status = OK
		return r
	}
}

// checkSbxAuth runs `sbx ls` — on a fresh install this fails until the user
// runs `sbx login`.
func checkSbxAuth(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "sbx auth"}
		enabled, _ := sandboxEnabled(root)
		if !enabled {
			r.Status = Skip
			r.Detail = "sandbox.enabled=false"
			return r
		}
		if _, err := execLookPath("sbx"); err != nil {
			r.Status = Skip
			r.Detail = "sbx not installed"
			return r
		}
		if _, err := runCombined(ctx, "sbx", "ls"); err != nil {
			r.Status = Fail
			r.Detail = "sbx ls failed — run `sbx login`"
			return r
		}
		r.Status = OK
		return r
	}
}

// checkSbxAgentCreds inspects env + sbx secrets for the credentials the
// selected coding agent needs.
func checkSbxAgentCreds(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "sbx agent creds"}
		enabled, _ := sandboxEnabled(root)
		if !enabled {
			r.Status = Skip
			r.Detail = "sandbox.enabled=false"
			return r
		}
		c, err := config.Read(root)
		if err != nil {
			r.Status = Fail
			r.Detail = err.Error()
			return r
		}
		switch c.CodingAgent {
		case "claude":
			if os.Getenv("ANTHROPIC_API_KEY") != "" {
				r.Status = OK
				r.Detail = "ANTHROPIC_API_KEY set in env"
				return r
			}
			if sbxSecretPresent(ctx, "anthropic") {
				r.Status = OK
				r.Detail = "sbx secret 'anthropic' present"
				return r
			}
			r.Status = Warn
			r.Detail = "ANTHROPIC_API_KEY not set and no 'anthropic' sbx secret"
		case "copilot":
			if os.Getenv("GH_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" {
				r.Status = OK
				r.Detail = "GH_TOKEN/GITHUB_TOKEN set"
				return r
			}
			if sbxSecretPresent(ctx, "github") {
				r.Status = OK
				r.Detail = "sbx secret 'github' present"
				return r
			}
			r.Status = Fail
			r.Detail = "no GH_TOKEN/GITHUB_TOKEN; run `echo \"$(gh auth token)\" | sbx secret set -g github`"
		default:
			r.Status = Skip
			r.Detail = "coding_agent not set"
		}
		return r
	}
}

// sbxSecretPresent returns true when `sbx secret ls` output mentions needle.
func sbxSecretPresent(ctx context.Context, needle string) bool {
	out, err := runCombined(ctx, "sbx", "secret", "ls")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(out), strings.ToLower(needle))
}

// checkSbxAutoflow verifies autoflow is installed inside a running sandbox.
func checkSbxAutoflow(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "sbx autoflow"}
		enabled, name := sandboxEnabled(root)
		if !enabled {
			r.Status = Skip
			r.Detail = "sandbox.enabled=false"
			return r
		}
		if _, err := execLookPath("sbx"); err != nil {
			r.Status = Skip
			r.Detail = "sbx not installed"
			return r
		}
		// Sandbox must be running.
		out, err := runCombined(ctx, "sbx", "ls")
		if err != nil || !containsSandbox(out, name) {
			r.Status = Skip
			r.Detail = "sandbox " + name + " not running"
			return r
		}
		if _, err := runCombined(ctx, "sbx", "exec", name, "--", "command", "-v", "autoflow"); err != nil {
			r.Status = Fail
			r.Detail = "autoflow not in sandbox — run `autoflow deliver sandbox bootstrap --name " + name + "`"
			return r
		}
		r.Status = OK
		r.Detail = "sandbox=" + name
		return r
	}
}

// containsSandbox scans sbx-ls output for a standalone token matching name.
func containsSandbox(out, name string) bool {
	for _, line := range strings.Split(out, "\n") {
		for _, tok := range strings.Fields(line) {
			if tok == name {
				return true
			}
		}
	}
	return false
}
