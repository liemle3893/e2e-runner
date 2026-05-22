package cli

import "github.com/spf13/cobra"

// newE2ECmd builds the `autoflow e2e ...` subtree — the YAML-driven test
// runner surface. Grouped under `e2e` to keep test-runner commands
// distinct from the delivery-workflow commands that sit at top level.
func newE2ECmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "e2e",
		Short: "YAML-driven multi-protocol E2E test runner",
		Long: `E2E test-runner commands.

Commands:
  run       Discover and run YAML test files
  list      List discovered test files and their metadata
  validate  Parse and validate YAML test files without running them
  init      Create a starter e2e.config.yaml in the current directory
  health    Check connectivity for all configured adapters
  test      Helpers for creating and managing test files
  doc       Show built-in documentation
  scaffold  Generate E2E test stubs for a ticket`,
	}
	cmd.AddCommand(
		newRunCmd(),
		newListCmd(),
		newValidateCmd(),
		newInitCmd(),
		newHealthCmd(),
		newTestCmd(),
		newDocCmd(),
		newAutoflowScaffoldCmd(),
	)
	return cmd
}
