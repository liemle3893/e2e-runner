package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/liemle3893/autoflow/internal/autoflow/scaffold"
	"github.com/liemle3893/autoflow/internal/autoflow/state"
)

func newAutoflowScaffoldCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "scaffold",
		Short: "Generate E2E test stubs for a ticket",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			ticket, _ := cmd.Flags().GetString("ticket")
			area, _ := cmd.Flags().GetString("area")
			count, _ := cmd.Flags().GetInt("count")
			results, err := scaffold.Generate(scaffold.Options{
				Root:   root,
				Ticket: ticket,
				Area:   area,
				Count:  count,
			})
			if err != nil {
				return err
			}
			created := 0
			for _, r := range results {
				if r.Created {
					fmt.Fprintf(cmd.OutOrStdout(), "  CREATED  %s\n", r.Path)
					created++
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  SKIP     %s (already exists)\n", r.Path)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Done: %d stub(s) created.\n", created)
			return nil
		},
	}
	c.Flags().String("ticket", "", "ticket key (required)")
	c.Flags().String("area", "", "test area subdirectory (required)")
	c.Flags().Int("count", 0, "number of stub files to create (required)")
	_ = c.MarkFlagRequired("ticket")
	_ = c.MarkFlagRequired("area")
	_ = c.MarkFlagRequired("count")
	return c
}
