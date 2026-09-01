// Package cli provides the top-level cobra command tree for the tryve binary.
package cli

import "github.com/spf13/cobra"

// NewRoot builds and returns the root cobra command for the tryve binary.
// It registers persistent flags shared by all sub-commands and wires all
// sub-commands into the tree.
func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:          "tryve",
		Short:        "tryve — YAML-driven multi-protocol test runner",
		SilenceUsage: true,
	}

	root.PersistentFlags().StringP("config", "c", "e2e.config.yaml", "config file path")
	root.PersistentFlags().StringP("env", "e", "local", "environment name")

	root.AddCommand(
		newRunCmd(),
		newValidateCmd(),
		newListCmd(),
		newHealthCmd(),
		newInitCmd(),
		newMigrateCmd(),
		newVersionCmd(version),
		newTestCmd(),
		newDocCmd(),
		newInstallCmd(),
	)

	return root
}
