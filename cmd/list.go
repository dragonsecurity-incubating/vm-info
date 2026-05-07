package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/digitalocean/go-libvirt"
	"github.com/dragonsecurity/vm-info/internal/virtcli"
	"github.com/spf13/cobra"
)

var (
	listAll      bool
	listInactive bool
	listName     bool
	listUUID     bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List domains (like virsh list)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			flags := libvirt.ConnectListDomainsActive
			if listAll {
				flags = libvirt.ConnectListDomainsActive | libvirt.ConnectListDomainsInactive
			} else if listInactive {
				flags = libvirt.ConnectListDomainsInactive
			}
			doms, _, err := l.ConnectListAllDomains(1024, flags)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			switch {
			case listName:
				for _, d := range doms {
					fmt.Fprintln(out, d.Name)
				}
				return nil
			case listUUID:
				for _, d := range doms {
					fmt.Fprintln(out, virtcli.FormatUUID(d.UUID))
				}
				return nil
			}

			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, " Id\tName\tState")
			fmt.Fprintln(tw, "----\t----\t-----")
			for _, d := range doms {
				id := "-"
				if d.ID > 0 {
					id = fmt.Sprintf("%d", d.ID)
				}
				state := "-"
				if s, _, _, _, _, err := l.DomainGetInfo(d); err == nil {
					state = virtcli.StateName(s)
				}
				fmt.Fprintf(tw, " %s\t%s\t%s\n", id, d.Name, state)
			}
			return tw.Flush()
		})
	},
}

func init() {
	listCmd.Flags().BoolVar(&listAll, "all", false, "include inactive domains")
	listCmd.Flags().BoolVar(&listInactive, "inactive", false, "show only inactive domains")
	listCmd.Flags().BoolVar(&listName, "name", false, "print only the names")
	listCmd.Flags().BoolVar(&listUUID, "uuid", false, "print only the UUIDs")
	rootCmd.AddCommand(listCmd)
}
