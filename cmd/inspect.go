package cmd

import (
	"context"
	"fmt"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

func runWithVM(args []string, fn func(ctx context.Context, p provider.Provider, vm provider.VM) error) error {
	return withProvider(func(ctx context.Context, p provider.Provider) error {
		vm, err := p.Lookup(ctx, args[0])
		if err != nil {
			return err
		}
		return fn(ctx, p, vm)
	})
}

var dominfoCmd = &cobra.Command{
	Use:   "dominfo <domain>",
	Short: "Print domain information (like virsh dominfo)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			info, err := p.Info(ctx, vm, nil)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Id:             %s\n", dashIfEmpty(info.ID))
			fmt.Fprintf(out, "Name:           %s\n", info.Name)
			fmt.Fprintf(out, "UUID:           %s\n", dashIfEmpty(info.UUID))
			if info.Node != "" {
				fmt.Fprintf(out, "Node:           %s\n", info.Node)
			}
			fmt.Fprintf(out, "Backend:        %s\n", info.Provider)
			fmt.Fprintf(out, "State:          %s\n", info.State)
			fmt.Fprintf(out, "CPU(s):         %d\n", info.VCPUs)
			fmt.Fprintf(out, "Max memory:     %d MiB\n", info.MaxMiB)
			fmt.Fprintf(out, "Used memory:    %d MiB\n", info.RAMMiB)
			return nil
		})
	},
}

var dumpxmlInactive bool

var dumpxmlCmd = &cobra.Command{
	Use:   "dumpxml <domain>",
	Short: "Print the domain XML (libvirt) or config dictionary (proxmox)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			s, err := p.Config(ctx, vm, dumpxmlInactive)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), s)
			if len(s) > 0 && s[len(s)-1] != '\n' {
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		})
	},
}

var domidCmd = &cobra.Command{
	Use:   "domid <domain>",
	Short: "Print the domain id",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			fmt.Fprintln(cmd.OutOrStdout(), dashIfEmpty(vm.ID))
			return nil
		})
	},
}

var domuuidCmd = &cobra.Command{
	Use:   "domuuid <domain>",
	Short: "Print the domain UUID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			fmt.Fprintln(cmd.OutOrStdout(), dashIfEmpty(vm.UUID))
			return nil
		})
	},
}

var domhostnameCmd = &cobra.Command{
	Use:   "domhostname <domain>",
	Short: "Print the guest hostname",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			h, err := p.Hostname(ctx, vm)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), h)
			return nil
		})
	},
}

var domstateCmd = &cobra.Command{
	Use:   "domstate <domain>",
	Short: "Print the domain state",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			s, err := p.State(ctx, vm)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), s)
			return nil
		})
	},
}

var vcpucountCmd = &cobra.Command{
	Use:   "vcpucount <domain>",
	Short: "Print the current vCPU count",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			info, err := p.Info(ctx, vm, nil)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), info.VCPUs)
			return nil
		})
	},
}

func init() {
	dumpxmlCmd.Flags().BoolVar(&dumpxmlInactive, "inactive", false, "show inactive (persistent) config (libvirt only)")
	rootCmd.AddCommand(dominfoCmd, dumpxmlCmd, domidCmd, domuuidCmd,
		domhostnameCmd, domstateCmd, vcpucountCmd)
}
