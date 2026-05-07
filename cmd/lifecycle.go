package cmd

import (
	"context"
	"fmt"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start <domain>",
	Short: "Start a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.Start(ctx, vm); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s started\n", vm.Name)
			return nil
		})
	},
}

var shutdownCmd = &cobra.Command{
	Use:   "shutdown <domain>",
	Short: "Gracefully shutdown a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.Shutdown(ctx, vm); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s is being shutdown\n", vm.Name)
			return nil
		})
	},
}

var destroyCmd = &cobra.Command{
	Use:   "destroy <domain>",
	Short: "Forcefully stop a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.Destroy(ctx, vm); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s destroyed\n", vm.Name)
			return nil
		})
	},
}

var rebootAcpi bool

var rebootCmd = &cobra.Command{
	Use:   "reboot <domain>",
	Short: "Reboot a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.Reboot(ctx, vm, provider.RebootOpts{ACPI: rebootAcpi}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s is being rebooted\n", vm.Name)
			return nil
		})
	},
}

var suspendCmd = &cobra.Command{
	Use:   "suspend <domain>",
	Short: "Suspend a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.Suspend(ctx, vm); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s suspended\n", vm.Name)
			return nil
		})
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume <domain>",
	Short: "Resume a suspended domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.Resume(ctx, vm); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s resumed\n", vm.Name)
			return nil
		})
	},
}

func init() {
	rebootCmd.Flags().BoolVar(&rebootAcpi, "acpi", false, "use ACPI power button instead of default (libvirt only)")
	mutating := []*cobra.Command{startCmd, shutdownCmd, destroyCmd, rebootCmd, suspendCmd, resumeCmd}
	for _, c := range mutating {
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations[MutatesAnnotation] = "true"
	}
	rootCmd.AddCommand(mutating...)
}
