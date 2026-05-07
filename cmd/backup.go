package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage VM backups",
	Long: `Manage VM backups.

  proxmox → vzdump (full implementation)
  libvirt → not implemented natively; use virt-backup, virsh backup-begin,
            or 'snapshot create' for point-in-time copies.`,
}

var backupListCmd = &cobra.Command{
	Use:   "list <domain>",
	Short: "List backups",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			bs, err := p.ListBackups(ctx, vm)
			if err != nil {
				return err
			}
			if len(bs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no backups)")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, " Name\tStorage\tFormat\tSize\tCreated\tNotes")
			fmt.Fprintln(tw, "------\t-------\t------\t----\t-------\t-----")
			for _, b := range bs {
				fmt.Fprintf(tw, " %s\t%s\t%s\t%s\t%s\t%s\n",
					dashIfEmpty(b.Name),
					dashIfEmpty(b.Storage),
					dashIfEmpty(b.Format),
					formatBytes(b.Size),
					formatTime(b.CreatedAt),
					dashIfEmpty(b.Notes))
			}
			return tw.Flush()
		})
	},
}

var (
	backupStorage  string
	backupMode     string
	backupCompress string
	backupNotes    string
)

var backupCreateCmd = &cobra.Command{
	Use:   "create <domain>",
	Short: "Create a backup (proxmox: vzdump)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			upid, err := p.CreateBackup(ctx, vm, provider.BackupOpts{
				Storage:  backupStorage,
				Mode:     backupMode,
				Compress: backupCompress,
				Notes:    backupNotes,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Backup task started on %s: %s\n", vm.Name, upid)
			return nil
		})
	},
}

var backupDeleteCmd = &cobra.Command{
	Use:   "delete <domain> <volid>",
	Short: "Delete a backup by full volid (e.g. local:backup/vzdump-qemu-100-...vma.zst)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.DeleteBackup(ctx, vm, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Backup %s deleted\n", args[1])
			return nil
		})
	},
}

func init() {
	backupCreateCmd.Flags().StringVar(&backupStorage, "storage", "",
		"target storage (proxmox; default: first backup-capable storage on the node)")
	backupCreateCmd.Flags().StringVar(&backupMode, "mode", "snapshot",
		"backup mode: snapshot, suspend, stop")
	backupCreateCmd.Flags().StringVar(&backupCompress, "compress", "zstd",
		"compression: zstd, gzip, lzo, none")
	backupCreateCmd.Flags().StringVar(&backupNotes, "notes", "", "backup notes")

	mutating := []*cobra.Command{backupCreateCmd, backupDeleteCmd}
	for _, c := range mutating {
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations[MutatesAnnotation] = "true"
	}

	backupCmd.AddCommand(backupListCmd, backupCreateCmd, backupDeleteCmd)
	rootCmd.AddCommand(backupCmd)
}
