package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/digitalocean/go-libvirt"
	"github.com/dragonsecurity/vm-info/internal/virtcli"
	"github.com/spf13/cobra"
)

var domblklistDetails bool

var domblklistCmd = &cobra.Command{
	Use:   "domblklist <domain>",
	Short: "List block devices (like virsh domblklist)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			xmlStr, err := l.DomainGetXMLDesc(d, 0)
			if err != nil {
				return err
			}
			dx, err := virtcli.ParseDomainXML(xmlStr)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			if domblklistDetails {
				fmt.Fprintln(tw, " Type\tDevice\tTarget\tSource")
				fmt.Fprintln(tw, "-----\t------\t------\t------")
				for _, dk := range dx.Devices.Disks {
					fmt.Fprintf(tw, " %s\t%s\t%s\t%s\n",
						dash(dk.Type), dash(dk.Device),
						dash(dk.Target.Dev), dash(dk.SourcePath()))
				}
			} else {
				fmt.Fprintln(tw, " Target\tSource")
				fmt.Fprintln(tw, "-------\t------")
				for _, dk := range dx.Devices.Disks {
					fmt.Fprintf(tw, " %s\t%s\n",
						dash(dk.Target.Dev), dash(dk.SourcePath()))
				}
			}
			return tw.Flush()
		})
	},
}

var domblkinfoCmd = &cobra.Command{
	Use:   "domblkinfo <domain> <target-or-source>",
	Short: "Print block device capacity / allocation",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			alloc, capacity, physical, err := l.DomainGetBlockInfo(d, args[1], 0)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Capacity:   %d\n", capacity)
			fmt.Fprintf(out, "Allocation: %d\n", alloc)
			fmt.Fprintf(out, "Physical:   %d\n", physical)
			return nil
		})
	},
}

func init() {
	domblklistCmd.Flags().BoolVar(&domblklistDetails, "details", false, "include type and device columns")
	rootCmd.AddCommand(domblklistCmd, domblkinfoCmd)
}
