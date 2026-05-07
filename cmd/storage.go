package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var domblklistDetails bool

var domblklistCmd = &cobra.Command{
	Use:   "domblklist <domain>",
	Short: "List block devices (like virsh domblklist)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			disks, err := p.Disks(ctx, vm)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			if domblklistDetails {
				fmt.Fprintln(tw, " Type\tDevice\tTarget\tSource")
				fmt.Fprintln(tw, "-----\t------\t------\t------")
				for _, d := range disks {
					fmt.Fprintf(tw, " %s\t%s\t%s\t%s\n",
						dashIfEmpty(d.Type), dashIfEmpty(d.Device),
						dashIfEmpty(d.Target), dashIfEmpty(d.Source))
				}
			} else {
				fmt.Fprintln(tw, " Target\tSource")
				fmt.Fprintln(tw, "-------\t------")
				for _, d := range disks {
					fmt.Fprintf(tw, " %s\t%s\n",
						dashIfEmpty(d.Target), dashIfEmpty(d.Source))
				}
			}
			return tw.Flush()
		})
	},
}

var domblkinfoCmd = &cobra.Command{
	Use:   "domblkinfo <domain> <target>",
	Short: "Print block device capacity / allocation",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			bi, err := p.BlockInfo(ctx, vm, args[1])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Capacity:   %d\n", bi.Capacity)
			fmt.Fprintf(out, "Allocation: %d\n", bi.Allocation)
			fmt.Fprintf(out, "Physical:   %d\n", bi.Physical)
			return nil
		})
	},
}

func init() {
	domblklistCmd.Flags().BoolVar(&domblklistDetails, "details", false, "include type and device columns")
	rootCmd.AddCommand(domblklistCmd, domblkinfoCmd)
}
