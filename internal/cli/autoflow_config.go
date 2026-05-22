package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/liemle3893/autoflow/internal/autoflow/config"
	"github.com/liemle3893/autoflow/internal/autoflow/state"
)

// newAutoflowConfigCmd builds the `autoflow deliver config` subtree for
// managing .autoflow/config.json.
func newAutoflowConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read or write .autoflow/config.json (coding agent + sandbox)",
	}

	setCmd := &cobra.Command{
		Use:   "set <field> <value>",
		Short: "Set a config field (coding_agent | sandbox.enabled | sandbox.name | sandbox.policy | sandbox.extra_mounts)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			if err := config.Set(root, args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Set %s=%s in %s\n", args[0], args[1], config.Path(root))
			return nil
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <field>",
		Short: "Print one field value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			val, err := config.Get(root, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), val)
			return nil
		},
	}

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Print the full config as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			out, err := config.Show(root)
			if err != nil {
				return err
			}
			_, _ = cmd.OutOrStdout().Write(out)
			return nil
		},
	}

	delCmd := &cobra.Command{
		Use:   "del [field]",
		Short: "Delete one field, or the whole config when no field is given",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			field := ""
			if len(args) == 1 {
				field = args[0]
			}
			return config.Del(root, field)
		},
	}

	cmd.AddCommand(setCmd, getCmd, showCmd, delCmd)
	return cmd
}
