package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/liemle3893/autoflow/internal/autoflow/deliver"
	"github.com/liemle3893/autoflow/internal/autoflow/e2e"
	"github.com/liemle3893/autoflow/internal/autoflow/report"
	"github.com/liemle3893/autoflow/internal/autoflow/state"
	"github.com/liemle3893/autoflow/internal/autoflow/worktree"
)

func newAutoflowDeliverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deliver",
		Short: "Jira-to-PR delivery workflow + supporting primitives",
	}
	cmd.AddCommand(
		// Public verbs
		newDeliverNextCmd(),
		newDeliverCompleteCmd(),
		newDeliverInitCmd(),
		newDeliverTimingsCmd(),
		// Internal API for skill agents
		newDeliverGateResultCmd(),
		newDeliverSetFieldCmd(),
		newDeliverCompleteStepCmd(),
		newDeliverE2ERoundCmd(),
		newDeliverReportCmd(),
		newDeliverVerifyGatesCmd(),
		newDeliverCommitTaskCmd(),
		// Workflow primitives (moved from top level in the 2026-05-23 reorg)
		newAutoflowJiraCmd(),
		newAutoflowWorktreeCmd(),
		newAutoflowSandboxCmd(),
		newAutoflowLoopStateCmd(),
		newAutoflowConfigCmd(),
		newAutoflowDoctorCmd(),
	)
	return cmd
}

func newDeliverNextCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "next",
		Short: "Return the JSON instruction for the current step",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			controller := deliver.NewController(root)
			instr, err := controller.Next(key)
			if err != nil {
				return err
			}
			out, err := deliver.MarshalIndent(instr)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	c.Flags().String("ticket", "", "ticket key (required)")
	_ = c.MarkFlagRequired("ticket")
	return c
}

func newDeliverCompleteCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "complete",
		Short: "Mark current step done and advance",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			title, _ := cmd.Flags().GetString("title")
			prURL, _ := cmd.Flags().GetString("pr-url")
			controller := deliver.NewController(root)
			resp, err := controller.Complete(key, deliver.CompleteOpts{Title: title, PRURL: prURL})
			if err != nil {
				return err
			}
			data, _ := json.Marshal(resp)
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	c.Flags().String("ticket", "", "ticket key (required)")
	c.Flags().String("title", "", "optional title extracted from step 1")
	c.Flags().String("pr-url", "", "optional PR URL extracted from step 11")
	_ = c.MarkFlagRequired("ticket")
	return c
}

func newDeliverInitCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "init",
		Short: "Seed workflow-progress.json for a ticket",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			wt, _ := cmd.Flags().GetString("worktree")
			br, _ := cmd.Flags().GetString("branch")
			controller := deliver.NewController(root)
			if err := controller.Init(key, wt, br); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"{\"status\":\"initialized\",\"ticket\":%q}\n", key)
			return nil
		},
	}
	c.Flags().String("ticket", "", "ticket key (required)")
	c.Flags().String("worktree", "", "worktree path (required)")
	c.Flags().String("branch", "", "branch name (required)")
	_ = c.MarkFlagRequired("ticket")
	_ = c.MarkFlagRequired("worktree")
	_ = c.MarkFlagRequired("branch")
	return c
}

// _gate-result is called from step 6's emitted bash command to record
// the build gate outcome without the LLM having to write JSON.
func newDeliverGateResultCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "_gate-result",
		Short:  "Internal: record build gate state from step 6",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			attempt, _ := cmd.Flags().GetInt("attempt")
			exitCode, _ := cmd.Flags().GetInt("exit-code")
			logFile, _ := cmd.Flags().GetString("log-file")
			if err := deliver.GateResult(root, key, attempt, exitCode, logFile); err != nil {
				return err
			}
			result := "pass"
			if exitCode != 0 {
				result = "fail"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "BUILD_GATE=%s\n", result)
			return nil
		},
	}
	c.Flags().String("ticket", "", "")
	c.Flags().Int("attempt", 0, "")
	c.Flags().Int("exit-code", 0, "")
	c.Flags().String("log-file", "", "")
	_ = c.MarkFlagRequired("ticket")
	_ = c.MarkFlagRequired("attempt")
	_ = c.MarkFlagRequired("exit-code")
	_ = c.MarkFlagRequired("log-file")
	return c
}

// _set-field is an internal shim for the progress-state.sh `set` verb,
// used by step 2's emitted bash commands.
func newDeliverSetFieldCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "_set-field",
		Short:  "Internal: set one whitelisted field on workflow-progress.json",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			field, _ := cmd.Flags().GetString("field")
			value, _ := cmd.Flags().GetString("value")
			return state.SetField(root, key, field, value)
		},
	}
	c.Flags().String("ticket", "", "")
	c.Flags().String("field", "", "")
	c.Flags().String("value", "", "")
	_ = c.MarkFlagRequired("ticket")
	_ = c.MarkFlagRequired("field")
	_ = c.MarkFlagRequired("value")
	return c
}

// _complete-step is the progress-state.sh `complete` verb.
func newDeliverCompleteStepCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "_complete-step",
		Short:  "Internal: mark a step complete in workflow-progress.json",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			step, _ := cmd.Flags().GetInt("step")
			return state.CompleteStep(root, key, step)
		},
	}
	c.Flags().String("ticket", "", "")
	c.Flags().Int("step", 0, "")
	_ = c.MarkFlagRequired("ticket")
	_ = c.MarkFlagRequired("step")
	return c
}

// _e2e-round runs one round of the step 7 loop.
func newDeliverE2ERoundCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "_e2e-round",
		Short:  "Internal: run one E2E round with state tracking",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			maxRounds, _ := cmd.Flags().GetInt("max-rounds")
			cfgPath, _ := cmd.Flags().GetString("config")
			env, _ := cmd.Flags().GetString("env")
			useLock, _ := cmd.Flags().GetBool("lock")

			progress, err := state.ReadProgress(root, key)
			if err != nil {
				return fmt.Errorf("read workflow-progress.json for %s: %w", key, err)
			}
			if progress == nil || progress.Worktree == "" {
				return fmt.Errorf("no workflow-progress.json with worktree set for %s (run `autoflow deliver init` first)", key)
			}

			local := e2e.LocalOptions{
				WorkDir:       progress.Worktree,
				TestSelection: "--tag " + key,
				ConfigPath:    cfgPath,
				Environment:   env,
				UseLock:       useLock,
				Stdout:        cmd.OutOrStdout(),
				Stderr:        cmd.ErrOrStderr(),
			}
			stateFile := filepath.Join(state.TicketStateDir(root, key), "e2e-fix-state.json")
			result, err := e2e.RunLoop(context.Background(), e2e.LoopOptions{
				Local:        local,
				Ticket:       key,
				StateFile:    stateFile,
				MaxRounds:    maxRounds,
				SkipDiagnose: true,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ROUND=%d STATUS=%s OUTPUT=%s\n",
				result.Round, result.Status, result.Output)
			return nil
		},
	}
	c.Flags().String("ticket", "", "")
	c.Flags().Int("max-rounds", 5, "")
	c.Flags().String("config", "e2e.config.yaml", "")
	c.Flags().String("env", "local", "")
	c.Flags().Bool("lock", false, "serialise runs via flock on <worktree>/.e2e-lock")
	_ = c.MarkFlagRequired("ticket")
	return c
}

// _report generates the three delivery markdown reports.
func newDeliverReportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "_report",
		Short:  "Internal: generate PR-BODY / JIRA-COMMENT / EXECUTION-REPORT",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			br, _ := cmd.Flags().GetString("branch")
			pr, _ := cmd.Flags().GetString("pr-url")

			cfg, _ := worktree.ReadConfig(root)
			baseBranch := "main"
			if cfg != nil {
				worktree.AutoDetect(cfg, root)
				baseBranch = cfg.BaseBranch
			}

			tdir := state.TicketDir(root, key)
			summaryDir := ""
			if p, _ := state.ReadProgress(root, key); p != nil {
				if p.ImplPlanDir != nil {
					summaryDir = *p.ImplPlanDir
				} else if p.GSDQuickID != nil {
					summaryDir = filepath.Join(root, ".planning", "quick", *p.GSDQuickID)
				}
			}

			out, err := report.Generate(report.Options{
				Ticket:     key,
				Branch:     br,
				PRURL:      pr,
				TicketDir:  tdir,
				BaseBranch: baseBranch,
				SummaryDir: summaryDir,
				BriefPath:  filepath.Join(tdir, "task-brief.md"),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Generated:\n  %s\n  %s\n  %s\n",
				out.PRBody, out.JiraComment, out.ExecutionReport)
			return nil
		},
	}
	c.Flags().String("ticket", "", "")
	c.Flags().String("branch", "", "")
	c.Flags().String("pr-url", "", "")
	_ = c.MarkFlagRequired("ticket")
	_ = c.MarkFlagRequired("branch")
	return c
}

// _commit-task is called by single-task executors at the end of their
// task. It serialises git add+commit across parallel executors sharing
// one worktree, and records the resulting SHA into plan-tasks.json.
func newDeliverCommitTaskCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "_commit-task",
		Short:  "Internal: stage files, commit under a file lock, record SHA",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			id, _ := cmd.Flags().GetString("task-id")
			msg, _ := cmd.Flags().GetString("message")
			filesCSV, _ := cmd.Flags().GetString("files")
			wt, _ := cmd.Flags().GetString("worktree")

			var files []string
			for _, f := range splitComma(filesCSV) {
				files = append(files, f)
			}

			// Flip status to running (idempotent) so parallel peers see
			// this task is in-flight and NextBatch skips it. If this
			// errors with ErrTaskAlreadyDone, another run already
			// committed — nothing to do.
			if err := state.MarkTaskRunning(root, key, id); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "NOTE: %v\n", err)
			}

			sha, err := deliver.CommitTask(deliver.CommitTaskRequest{
				Root:     root,
				Key:      key,
				TaskID:   id,
				Message:  msg,
				Files:    files,
				Worktree: wt,
			})
			if err != nil {
				_ = state.MarkTaskFailed(root, key, id, err.Error())
				return err
			}
			if sha == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "TASK_COMMIT=(empty) TASK_ID=%s\n", id)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "TASK_COMMIT=%s TASK_ID=%s\n", sha, id)
			}
			return nil
		},
	}
	c.Flags().String("ticket", "", "")
	c.Flags().String("task-id", "", "")
	c.Flags().String("message", "", "")
	c.Flags().String("files", "", "comma-separated list (relative to worktree; empty = all changes)")
	c.Flags().String("worktree", "", "worktree path")
	_ = c.MarkFlagRequired("ticket")
	_ = c.MarkFlagRequired("task-id")
	_ = c.MarkFlagRequired("message")
	_ = c.MarkFlagRequired("worktree")
	return c
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0, 4)
	for _, p := range splitAndTrim(s, ",") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitAndTrim(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// _verify-gates runs the structural validator against the two canonical
// loop state files. Any FAIL issue is reported; warnings are printed but
// still exit 0.
func newDeliverVerifyGatesCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "_verify-gates",
		Short:  "Internal: validate coverage/e2e state files",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			key, _ := cmd.Flags().GetString("ticket")
			stateDir := state.TicketStateDir(root, key)
			var anyFail bool
			for _, name := range []string{"coverage-review-state.json", "e2e-fix-state.json"} {
				path := filepath.Join(stateDir, name)
				if _, err := os.Stat(path); err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "SKIP: %s (not present)\n", name)
					continue
				}
				rounds, issues, err := state.VerifyLoopStateFile(path)
				if err != nil {
					return err
				}
				for _, iss := range issues {
					if iss.Severity == "FAIL" {
						anyFail = true
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s — %s\n", iss.Severity, name, iss.Message)
				}
				if len(issues) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "OK: %s (%d rounds)\n", name, rounds)
				}
			}
			if anyFail {
				return fmt.Errorf("state validation failed")
			}
			return nil
		},
	}
	c.Flags().String("ticket", "", "")
	_ = c.MarkFlagRequired("ticket")
	return c
}
