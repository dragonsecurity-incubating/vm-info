package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var domifaddrSource string

var domifaddrCmd = &cobra.Command{
	Use:   "domifaddr <domain>",
	Short: "Show interface addresses (like virsh domifaddr)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			ifs, err := p.Interfaces(ctx, vm, domifaddrSource)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, " Name\tMAC address\tProtocol\tAddress")
			fmt.Fprintln(tw, "------\t-----------\t--------\t-------")
			for _, iface := range ifs {
				if iface.Addr == "" {
					fmt.Fprintf(tw, " %s\t%s\t-\t-\n", iface.Name, dashIfEmpty(iface.MAC))
					continue
				}
				fmt.Fprintf(tw, " %s\t%s\t%s\t%s/%d\n",
					iface.Name, dashIfEmpty(iface.MAC), iface.Protocol, iface.Addr, iface.Prefix)
			}
			return tw.Flush()
		})
	},
}

var domiflistCmd = &cobra.Command{
	Use:   "domiflist <domain>",
	Short: "List domain network interfaces (like virsh domiflist)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			devs, err := p.NetDevices(ctx, vm)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, " Interface\tType\tSource\tModel\tMAC")
			fmt.Fprintln(tw, "----------\t----\t------\t-----\t---")
			for _, d := range devs {
				fmt.Fprintf(tw, " %s\t%s\t%s\t%s\t%s\n",
					dashIfEmpty(d.Target), dashIfEmpty(d.Type),
					dashIfEmpty(d.Source), dashIfEmpty(d.Model),
					dashIfEmpty(d.MAC))
			}
			return tw.Flush()
		})
	},
}

func init() {
	domifaddrCmd.Flags().StringVar(&domifaddrSource, "source", "lease",
		"address source: lease, agent, or arp (libvirt); agent only on proxmox")
	rootCmd.AddCommand(domifaddrCmd, domiflistCmd)
}
