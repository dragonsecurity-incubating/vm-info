package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/digitalocean/go-libvirt"
	"github.com/dragonsecurity/vm-info/internal/virtcli"
	"github.com/spf13/cobra"
)

var domifaddrSource string

var domifaddrCmd = &cobra.Command{
	Use:   "domifaddr <domain>",
	Short: "Show interface addresses (like virsh domifaddr)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			var src libvirt.DomainInterfaceAddressesSource
			switch domifaddrSource {
			case "lease":
				src = libvirt.DomainInterfaceAddressesSrcLease
			case "agent":
				src = libvirt.DomainInterfaceAddressesSrcAgent
			case "arp":
				src = libvirt.DomainInterfaceAddressesSrcArp
			default:
				return fmt.Errorf("source must be one of: lease, agent, arp")
			}
			ifs, err := l.DomainInterfaceAddresses(d, uint32(src), 0)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, " Name\tMAC address\tProtocol\tAddress")
			fmt.Fprintln(tw, "------\t-----------\t--------\t-------")
			for _, iface := range ifs {
				mac := "-"
				if len(iface.Hwaddr) > 0 {
					mac = iface.Hwaddr[0]
				}
				if len(iface.Addrs) == 0 {
					fmt.Fprintf(tw, " %s\t%s\t-\t-\n", iface.Name, mac)
					continue
				}
				for _, a := range iface.Addrs {
					proto := "ipv4"
					if libvirt.IPAddrType(a.Type) == libvirt.IPAddrTypeIpv6 {
						proto = "ipv6"
					}
					fmt.Fprintf(tw, " %s\t%s\t%s\t%s/%d\n",
						iface.Name, mac, proto, a.Addr, a.Prefix)
				}
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
			fmt.Fprintln(tw, " Interface\tType\tSource\tModel\tMAC")
			fmt.Fprintln(tw, "----------\t----\t------\t-----\t---")
			for _, iface := range dx.Devices.Interfaces {
				fmt.Fprintf(tw, " %s\t%s\t%s\t%s\t%s\n",
					dash(iface.Target.Dev),
					dash(iface.Type),
					iface.SourceLabel(),
					dash(iface.Model.Type),
					dash(iface.MAC.Address))
			}
			return tw.Flush()
		})
	},
}

func init() {
	domifaddrCmd.Flags().StringVar(&domifaddrSource, "source", "lease",
		"address source: lease, agent, or arp")
	rootCmd.AddCommand(domifaddrCmd, domiflistCmd)
}
