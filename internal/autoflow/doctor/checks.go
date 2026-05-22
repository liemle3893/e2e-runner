package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/liemle3893/autoflow/internal/autoflow/jira"
	"github.com/liemle3893/autoflow/internal/autoflow/worktree"
)

// checkBinary verifies that name is runnable. description is shown
// when the check fails.
func checkBinary(name, description string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: name + " on PATH"}
		if _, err := exec.LookPath(name); err != nil {
			r.Status = Fail
			r.Detail = "not found (" + description + ")"
			return r
		}
		r.Status = OK
		return r
	}
}

// checkGH runs `gh auth status`. The command exits 0 only when at least
// one auth context is valid, so a non-zero exit is a clean failure.
func checkGH(ctx context.Context) Result {
	r := Result{Name: "gh auth"}
	if _, err := exec.LookPath("gh"); err != nil {
		r.Status = Fail
		r.Detail = "gh not installed"
		return r
	}
	cmd := exec.CommandContext(ctx, "gh", "auth", "status")
	// gh writes its banner to stderr; we only care about exit code.
	if err := cmd.Run(); err != nil {
		r.Status = Fail
		r.Detail = "gh auth status failed — run `gh auth login`"
		return r
	}
	r.Status = OK
	return r
}

// checkJIRAToken checks only that the env var is set — the round-trip
// check is checkJIRAReachable.
func checkJIRAToken(ctx context.Context) Result {
	r := Result{Name: "JIRA_API_TOKEN"}
	if os.Getenv("JIRA_API_TOKEN") == "" {
		r.Status = Fail
		r.Detail = "env var not set — export it or the agent will hunt for credentials"
		return r
	}
	r.Status = OK
	return r
}

// checkJIRAConfig validates the cache shape. Missing file is a FAIL
// because Jira calls in the workflow need it — in contrast to bootstrap
// which is only a WARN.
func checkJIRAConfig(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "jira-config.json"}
		c, err := jira.Read(root)
		if err != nil {
			if errors.Is(err, jira.ErrNoConfig) {
				r.Status = Fail
				r.Detail = "not cached — run `autoflow deliver jira config set ...`"
				return r
			}
			r.Status = Fail
			r.Detail = err.Error()
			return r
		}
		var missing []string
		if c.CloudID == "" {
			missing = append(missing, "cloudId")
		}
		if c.SiteURL == "" {
			missing = append(missing, "siteUrl")
		}
		if c.ProjectKey == "" {
			missing = append(missing, "projectKey")
		}
		if c.Email == "" {
			missing = append(missing, "email")
		}
		if len(missing) > 0 {
			r.Status = Fail
			r.Detail = fmt.Sprintf("missing fields: %v", missing)
			return r
		}
		r.Status = OK
		return r
	}
}

// checkJIRAReachable pings /rest/api/3/myself using cached creds + env
// token. Any error short of a clean 200 response is a fail.
func checkJIRAReachable(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "Jira reachable"}
		creds, err := jira.ResolveCredentials(root)
		if err != nil {
			r.Status = Fail
			r.Detail = err.Error()
			return r
		}
		client := jira.NewClient(creds)
		acct, err := client.Myself(ctx)
		if err != nil {
			r.Status = Fail
			r.Detail = err.Error()
			return r
		}
		r.Status = OK
		r.Detail = "accountId=" + acct
		return r
	}
}

// checkBootstrap is a soft check — bootstrap.json is nice to have but
// not required unless the user plans to run `autoflow deliver worktree
// bootstrap`.
func checkBootstrap(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "bootstrap.json"}
		if _, err := os.Stat(worktree.ConfigPath(root)); errors.Is(err, os.ErrNotExist) {
			r.Status = Warn
			r.Detail = "not configured — run the autoflow-settings skill when you need worktree bootstrap"
			return r
		} else if err != nil {
			r.Status = Fail
			r.Detail = err.Error()
			return r
		}
		r.Status = OK
		return r
	}
}

// checkSkillsInstalled verifies .claude/skills/autoflow-deliver/SKILL.md
// exists. That one file is a reliable signal that `autoflow install --autoflow`
// has been run.
func checkSkillsInstalled(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "skills installed"}
		path := filepath.Join(root, ".claude", "skills", "autoflow-deliver", "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			r.Status = Fail
			r.Detail = "run `autoflow install --autoflow` in this repo"
			return r
		}
		r.Status = OK
		return r
	}
}

// checkAgentsInstalled checks the jira-fetcher agent as a canonical
// marker. If agents drift out of sync in future, expand this to check
// all 14 filenames.
func checkAgentsInstalled(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "agents installed"}
		path := filepath.Join(root, ".claude", "agents", "autoflow-jira-fetcher.md")
		if _, err := os.Stat(path); err != nil {
			r.Status = Fail
			r.Detail = "run `autoflow install --autoflow` in this repo"
			return r
		}
		r.Status = OK
		return r
	}
}

// checkNoLegacyScripts warns when the winx-autoflow bash-installer
// dropped scripts into .claude/scripts/autoflow/. They will collide with
// the Go port's command shapes in the SKILL.md prompts.
func checkNoLegacyScripts(root string) Checker {
	return func(ctx context.Context) Result {
		r := Result{Name: "no legacy scripts"}
		path := filepath.Join(root, ".claude", "scripts", "autoflow")
		if _, err := os.Stat(path); err == nil {
			r.Status = Warn
			r.Detail = "`.claude/scripts/autoflow/` exists — remove it and re-run `autoflow install --autoflow`"
			return r
		}
		r.Status = OK
		return r
	}
}
