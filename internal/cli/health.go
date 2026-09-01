package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/config"
)

// newHealthCmd constructs the `health` sub-command which checks connectivity
// for every adapter configured in the active environment.
func newHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check connectivity for all configured adapters",
		Args:  cobra.NoArgs,
		RunE:  healthCmdHandler,
	}
}

// healthCmdHandler implements the `health` command execution logic.
func healthCmdHandler(cmd *cobra.Command, _ []string) error {
	cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
	envName, _ := cmd.Root().PersistentFlags().GetString("env")

	cfg, err := config.Load(cfgPath, envName)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	reg := adapter.BuildRegistry(adapter.RegistryOptions{
		BaseURL:   cfg.Environment.BaseURL,
		Adapters:  cfg.Environment.Adapters,
		ConfigDir: adapter.ConfigDirOf(cfgPath),
		Compat:    cfg.Compat,
		Warn:      os.Stderr,
	})

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	out := cmd.OutOrStdout()
	hasFailure := false

	for _, name := range reg.Names() {
		a, getErr := reg.Get(ctx, name)
		if getErr != nil {
			fmt.Fprintf(out, "FAIL  %-12s connect error: %v\n", name, getErr)
			hasFailure = true
			continue
		}
		if hErr := a.Health(ctx); hErr != nil {
			fmt.Fprintf(out, "FAIL  %-12s health error: %v\n", name, hErr)
			hasFailure = true
			continue
		}
		fmt.Fprintf(out, "OK    %-12s\n", name)
	}

	if hasFailure {
		os.Exit(1)
	}
	return nil
}
