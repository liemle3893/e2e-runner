// Package cli provides the top-level cobra command tree for the autoflow binary.
package cli

import "github.com/spf13/cobra"

// NewRoot builds and returns the root cobra command for the autoflow binary.
// Two visible products: the YAML-driven E2E test runner (`e2e`) and the
// Jira-to-PR delivery workflow (`deliver`). Workflow primitives (jira,
// worktree, sandbox, loop-state, config, doctor) live as children of
// `deliver`. `install` and `version` are cross-cutting top-level peers.
func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:          "autoflow",
		Short:        "autoflow — Jira-to-PR delivery workflow + YAML-driven E2E test runner",
		SilenceUsage: true,
	}

	root.PersistentFlags().StringP("config", "c", "e2e.config.yaml", "config file path")
	root.PersistentFlags().StringP("env", "e", "local", "environment name")

	root.AddCommand(
		// E2E test-runner subtree
		newE2ECmd(),
		// Delivery workflow umbrella (owns jira/worktree/sandbox/loop-state/config/doctor as children)
		newAutoflowDeliverCmd(),
		// Cross-cutting
		newInstallCmd(),
		newVersionCmd(version),
	)

	// Eagerly initialise cobra's built-in help and completion commands so
	// that root.Commands() returns them before Execute() is called (used by
	// layout tests and tab-completion generators).
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	return root
}
