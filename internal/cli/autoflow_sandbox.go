package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/liemle3893/autoflow/internal/autoflow/config"
	"github.com/liemle3893/autoflow/internal/autoflow/sandbox"
	"github.com/liemle3893/autoflow/internal/autoflow/state"
)

// sandboxVersion is populated from main() at startup via SetSandboxHostVersion
// so the sandbox package can install a matching binary.
var sandboxHostVersion = "dev"

// SetSandboxHostVersion is invoked from cmd/autoflow/main.go so the CLI's
// version string (set via -ldflags) is available to `autoflow deliver sandbox
// bootstrap`.
func SetSandboxHostVersion(v string) {
	if v != "" {
		sandboxHostVersion = v
	}
}

func newAutoflowSandboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sandbox",
		Short: "Manage the per-repo sbx sandbox used by coding agents",
	}
	cmd.AddCommand(newAutoflowSandboxBootstrapCmd(), newAutoflowSandboxStatusCmd())
	return cmd
}

// resolveSandboxName returns --name when set, else sandbox.name from config,
// else the basename of the repo root.
func resolveSandboxName(cmd *cobra.Command, root string) string {
	if v, _ := cmd.Flags().GetString("name"); v != "" {
		return v
	}
	if c, err := config.Read(root); err == nil && c.Sandbox.Name != "" {
		return c.Sandbox.Name
	}
	return filepath.Base(root)
}

func newAutoflowSandboxBootstrapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Install the host autoflow binary into the sandbox",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			name := resolveSandboxName(cmd, root)
			hostBin, _ := os.Executable()
			return sandbox.Bootstrap(context.Background(), cmd.OutOrStdout(), sandbox.BootstrapOpts{
				Name:       name,
				HostVer:    sandboxHostVersion,
				HostBinary: hostBin,
			})
		},
	}
	cmd.Flags().String("name", "", "sandbox name (defaults to sandbox.name from config, or repo basename)")
	return cmd
}

func newAutoflowSandboxStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show host vs. sandbox autoflow version and arch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := state.RepoRoot()
			if err != nil {
				return err
			}
			name := resolveSandboxName(cmd, root)
			host, sb, arch, err := sandbox.Status(context.Background(), name)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Sandbox:     %s\n", name)
			fmt.Fprintf(out, "Arch:        %s\n", emptyDash(arch))
			fmt.Fprintf(out, "Host ver:    %s\n", emptyDash(host))
			fmt.Fprintf(out, "Sandbox ver: %s\n", emptyDash(sb))
			return nil
		},
	}
	cmd.Flags().String("name", "", "sandbox name (defaults to sandbox.name from config, or repo basename)")
	return cmd
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
