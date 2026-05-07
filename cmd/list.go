package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var (
	listName bool
	listUUID bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List domains (like virsh list)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withProvider(func(ctx context.Context, p provider.Provider) error {
			vms, err := p.List(ctx)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch {
			case listName:
				for _, vm := range vms {
					fmt.Fprintln(out, vm.Name)
				}
				return nil
			case listUUID:
				for _, vm := range vms {
					fmt.Fprintln(out, vm.UUID)
				}
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, " Id\tName\tState")
			fmt.Fprintln(tw, "----\t----\t-----")
			for _, vm := range vms {
				state, err := p.State(ctx, vm)
				if err != nil {
					state = "-"
				}
				fmt.Fprintf(tw, " %s\t%s\t%s\n", dashIfEmpty(vm.ID), vm.Name, state)
			}
			return tw.Flush()
		})
	},
}

func init() {
	listCmd.Flags().BoolVar(&listName, "name", false, "print only the names")
	listCmd.Flags().BoolVar(&listUUID, "uuid", false, "print only the UUIDs")
	rootCmd.AddCommand(listCmd)
}
