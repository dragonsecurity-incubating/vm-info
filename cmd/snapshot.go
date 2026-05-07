package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage VM snapshots",
	Long: `Manage point-in-time VM snapshots.

Snapshots are backend-native:
  libvirt → virsh snapshot-* equivalents (DomainSnapshot* APIs)
  proxmox → /nodes/{node}/qemu/{vmid}/snapshot* endpoints`,
}

var snapshotListCmd = &cobra.Command{
	Use:   "list <domain>",
	Short: "List snapshots",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			snaps, err := p.ListSnapshots(ctx, vm)
			if err != nil {
				return err
			}
			if len(snaps) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no snapshots)")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, " Name\tCreated\tState\tParent\tDescription")
			fmt.Fprintln(tw, "------\t-------\t-----\t------\t-----------")
			for _, s := range snaps {
				marker := ""
				if s.Current {
					marker = " *"
				}
				fmt.Fprintf(tw, " %s%s\t%s\t%s\t%s\t%s\n",
					s.Name, marker,
					formatTime(s.CreatedAt),
					dashIfEmpty(s.State),
					dashIfEmpty(s.Parent),
					dashIfEmpty(s.Description))
			}
			return tw.Flush()
		})
	},
}

var (
	snapDescription string
	snapMemory      bool
)

var snapshotCreateCmd = &cobra.Command{
	Use:   "create <domain> <name>",
	Short: "Create a snapshot",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			err := p.CreateSnapshot(ctx, vm, args[1], provider.SnapshotOpts{
				Description: snapDescription,
				Memory:      snapMemory,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Snapshot %s created on %s\n", args[1], vm.Name)
			return nil
		})
	},
}

var snapshotDeleteCmd = &cobra.Command{
	Use:   "delete <domain> <name>",
	Short: "Delete a snapshot",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.DeleteSnapshot(ctx, vm, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Snapshot %s deleted from %s\n", args[1], vm.Name)
			return nil
		})
	},
}

var snapshotRevertCmd = &cobra.Command{
	Use:     "revert <domain> <name>",
	Aliases: []string{"rollback", "restore"},
	Short:   "Revert to a snapshot",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.RevertSnapshot(ctx, vm, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Reverted %s to snapshot %s\n", vm.Name, args[1])
			return nil
		})
	},
}

func formatTime(t time.Time) string {
	if t.IsZero() || t.Unix() <= 0 {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func init() {
	snapshotCreateCmd.Flags().StringVarP(&snapDescription, "description", "d", "", "snapshot description")
	snapshotCreateCmd.Flags().BoolVar(&snapMemory, "memory", false, "include memory state (proxmox: vmstate=1)")

	mutating := []*cobra.Command{snapshotCreateCmd, snapshotDeleteCmd, snapshotRevertCmd}
	for _, c := range mutating {
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations[MutatesAnnotation] = "true"
	}

	snapshotCmd.AddCommand(snapshotListCmd, snapshotCreateCmd, snapshotDeleteCmd, snapshotRevertCmd)
	rootCmd.AddCommand(snapshotCmd)
}
