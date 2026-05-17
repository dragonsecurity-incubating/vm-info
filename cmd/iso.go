package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var isoCmd = &cobra.Command{
	Use:   "iso",
	Short: "Mount and unmount ISO images",
	Long: `Mount or unmount ISO media in a VM's cdrom slot.

  proxmox → PUT /nodes/{node}/qemu/{vmid}/config with ideN/scsiN media=cdrom
  libvirt → DomainUpdateDeviceFlags / DomainAttachDeviceFlags with cdrom XML`,
}

var isoListCmd = &cobra.Command{
	Use:   "list <domain>",
	Short: "List cdrom slots and currently-attached media",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			disks, err := p.Disks(ctx, vm)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "SLOT\tBUS\tSOURCE")
			fmt.Fprintln(tw, "----\t---\t------")
			any := false
			for _, d := range disks {
				if d.Device != "cdrom" {
					continue
				}
				any = true
				src := d.Source
				if src == "none" {
					src = ""
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Target, dashIfEmpty(d.Bus), dashIfEmpty(src))
			}
			if !any {
				fmt.Fprintln(cmd.OutOrStdout(), "(no cdrom slots)")
				return nil
			}
			return tw.Flush()
		})
	},
}

var isoAttachCmd = &cobra.Command{
	Use:   "attach <domain> <slot> <source>",
	Short: "Mount an ISO image",
	Long: `Mount an ISO into a cdrom slot. The slot is created if it doesn't exist yet
(both backends support this, though Proxmox is more permissive about slot keys).

  proxmox: slot=ideN/scsiN/sataN, source=storage:iso/file.iso
           vm-info --rw iso attach 100 ide2 local:iso/debian-12.iso
  libvirt: slot=hdc/sda/vdc, source=host filesystem path
           vm-info --rw iso attach cp1 hdc /var/lib/libvirt/isos/debian.iso`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.AttachISO(ctx, vm, args[1], args[2]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Attached %s to %s on %s\n", args[2], args[1], vm.Name)
			return nil
		})
	},
}

var isoEjectCmd = &cobra.Command{
	Use:     "eject <domain> <slot>",
	Aliases: []string{"detach"},
	Short:   "Eject media from a cdrom slot (keeps the slot attached)",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.EjectISO(ctx, vm, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Ejected media from %s on %s\n", args[1], vm.Name)
			return nil
		})
	},
}

func init() {
	for _, c := range []*cobra.Command{isoAttachCmd, isoEjectCmd} {
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations[MutatesAnnotation] = "true"
	}
	isoCmd.AddCommand(isoListCmd, isoAttachCmd, isoEjectCmd)
	rootCmd.AddCommand(isoCmd)
}
