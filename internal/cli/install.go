package cli

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	assets "github.com/liemle3893/autoflow"
	"github.com/liemle3893/autoflow/internal/autoflow/config"
)

// newInstallCmd constructs the `install` sub-command which copies bundled
// skills, documentation references and (optionally) the autoflow agents
// + skills into the user's project under `.claude/`.
func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Claude Code skills into the current project",
		Long: `Install the bundled Claude Code assets into the current project.

  --skills     autoflow-cli skill into .claude/skills/autoflow-cli/
  --autoflow   autoflow skills + agents into .claude/{skills,agents}/,
               and auto-clean any legacy .claude/scripts/autoflow/ dir.

Both flags may be combined. Without flags, prints usage.`,
		Args: cobra.NoArgs,
		RunE: installCmdHandler,
	}

	cmd.Flags().Bool("skills", false, "install autoflow-cli skill")
	cmd.Flags().Bool("autoflow", false, "install autoflow skills + agents")
	return cmd
}

// installCmdHandler implements the `install` command execution logic.
func installCmdHandler(cmd *cobra.Command, _ []string) error {
	skills, _ := cmd.Flags().GetBool("skills")
	autoflow, _ := cmd.Flags().GetBool("autoflow")
	if !skills && !autoflow {
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "Usage: autoflow install [--skills] [--autoflow]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Options:")
		fmt.Fprintln(out, "  --skills     install autoflow-cli skill to .claude/skills/autoflow-cli/")
		fmt.Fprintln(out, "  --autoflow   install autoflow skills + agents to .claude/{skills,agents}/")
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("install: cannot determine working directory: %w", err)
	}

	if skills {
		if err := installSkills(cmd, cwd); err != nil {
			return err
		}
	}
	if autoflow {
		if err := installAutoflow(cmd, cwd); err != nil {
			return err
		}
	}
	return nil
}

// installSkills copies the autoflow-cli skill + doc references (previous
// default behaviour of `--skills`).
func installSkills(cmd *cobra.Command, cwd string) error {
	destDir := filepath.Join(cwd, ".claude", "skills", "autoflow-cli")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("install: creating %s: %w", destDir, err)
	}
	if err := copyEmbedDir(assets.SkillsFS, "skills/autoflow-cli", destDir, nil); err != nil {
		return fmt.Errorf("install: copying skill bundle: %w", err)
	}
	refsDir := filepath.Join(destDir, "references")
	skip := map[string]struct{}{"docs/sections/index.json": {}}
	if err := copyEmbedDir(assets.DocsSectionsFS, "docs/sections", refsDir, skip); err != nil {
		return fmt.Errorf("install: copying documentation references: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Skills installed to %s\n", destDir)
	return nil
}

// installAutoflow drops the autoflow skills + agents into .claude/ and
// removes any legacy bash-script install directory.
func installAutoflow(cmd *cobra.Command, cwd string) error {
	skillsDst := filepath.Join(cwd, ".claude", "skills")
	agentsDst := filepath.Join(cwd, ".claude", "agents")
	if err := os.MkdirAll(skillsDst, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(agentsDst, 0o755); err != nil {
		return err
	}

	// Purge any previously installed autoflow skills/agents before
	// re-copying. Stale copies can carry obsolete references (e.g. old
	// mcp__atlassian__* calls) and would otherwise silently survive an
	// upgrade because individual files aren't overwritten by name here.
	if err := purgeAutoflowInstall(cmd, skillsDst, agentsDst); err != nil {
		return err
	}

	if err := copyEmbedDir(assets.AutoflowSkillsFS, "skills/autoflow", skillsDst, nil); err != nil {
		return fmt.Errorf("install: copying autoflow skills: %w", err)
	}
	if err := copyEmbedDir(assets.AutoflowAgentsFS, "agents/autoflow", agentsDst, nil); err != nil {
		return fmt.Errorf("install: copying autoflow agents: %w", err)
	}

	// Auto-clean the legacy bash-script layout so stale paths in old
	// SKILL.md instances cannot be resolved and silently re-used.
	legacy := filepath.Join(cwd, ".claude", "scripts", "autoflow")
	if _, err := os.Stat(legacy); err == nil {
		if err := os.RemoveAll(legacy); err != nil {
			return fmt.Errorf("install: removing legacy %s: %w", legacy, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed legacy %s (replaced by autoflow subcommands)\n", legacy)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Autoflow skills installed under %s\n", skillsDst)
	fmt.Fprintf(cmd.OutOrStdout(), "Autoflow agents installed under %s\n", agentsDst)

	// Non-interactive next-step guidance (idempotent — printed on every
	// install so the user sees it even after adjusting config manually).
	out := cmd.OutOrStdout()
	cfgPath := config.Path(cwd)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		fmt.Fprintln(out, "Next: autoflow deliver config set coding_agent claude|copilot")
	}
	c, _ := config.Read(cwd)
	if c != nil && c.CodingAgent != "" {
		if _, err := exec.LookPath("sbx"); err == nil {
			slug := filepath.Base(cwd)
			fmt.Fprintf(out,
				"Suggested: sbx run %s --name %s . && autoflow deliver sandbox bootstrap --name %s\n",
				c.CodingAgent, slug, slug)
		}
	}
	return nil
}

// purgeAutoflowInstall removes every `.claude/skills/autoflow-*` directory
// and every `.claude/agents/autoflow-*.md` file that the embed FS would
// install, so stale copies (e.g. referencing old MCP tools) don't survive
// an upgrade. Unrelated skills/agents under those roots are left alone.
func purgeAutoflowInstall(cmd *cobra.Command, skillsDst, agentsDst string) error {
	skillNames, err := listEmbedChildren(assets.AutoflowSkillsFS, "skills/autoflow")
	if err != nil {
		return fmt.Errorf("install: listing bundled autoflow skills: %w", err)
	}
	for _, name := range skillNames {
		target := filepath.Join(skillsDst, name)
		if _, err := os.Stat(target); err == nil {
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("install: removing %s: %w", target, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed stale %s\n", target)
		}
	}
	agentNames, err := listEmbedChildren(assets.AutoflowAgentsFS, "agents/autoflow")
	if err != nil {
		return fmt.Errorf("install: listing bundled autoflow agents: %w", err)
	}
	for _, name := range agentNames {
		target := filepath.Join(agentsDst, name)
		if _, err := os.Stat(target); err == nil {
			if err := os.Remove(target); err != nil {
				return fmt.Errorf("install: removing %s: %w", target, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed stale %s\n", target)
		}
	}
	return nil
}

// listEmbedChildren returns the immediate child names (files or directories)
// of `root` inside `src`. It does NOT recurse.
func listEmbedChildren(src fs.FS, root string) ([]string, error) {
	entries, err := fs.ReadDir(src, root)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}

// copyEmbedDir recursively copies an embedded directory tree rooted at root
// into destDir on disk. Files matching any entry in skip (keyed by their
// embed path) are omitted.
func copyEmbedDir(src fs.FS, root, destDir string, skip map[string]struct{}) error {
	return fs.WalkDir(src, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if _, skipped := skip[path]; skipped {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))

		if d.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", target, err)
			}
			return nil
		}

		data, err := fs.ReadFile(src, path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating parent of %s: %w", target, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
		return nil
	})
}
