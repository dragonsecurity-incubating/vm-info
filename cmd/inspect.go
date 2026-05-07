package cmd

import (
	"fmt"

	"github.com/digitalocean/go-libvirt"
	"github.com/dragonsecurity/vm-info/internal/virtcli"
	"github.com/spf13/cobra"
)

func lookup(l *libvirt.Libvirt, name string) (libvirt.Domain, error) {
	return l.DomainLookupByName(name)
}

var dominfoCmd = &cobra.Command{
	Use:   "dominfo <domain>",
	Short: "Print domain information (like virsh dominfo)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			state, maxMem, mem, vcpus, cpuTime, err := l.DomainGetInfo(d)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			id := "-"
			if d.ID > 0 {
				id = fmt.Sprintf("%d", d.ID)
			}
			fmt.Fprintf(out, "Id:             %s\n", id)
			fmt.Fprintf(out, "Name:           %s\n", d.Name)
			fmt.Fprintf(out, "UUID:           %s\n", virtcli.FormatUUID(d.UUID))
			fmt.Fprintf(out, "State:          %s\n", virtcli.StateName(state))
			fmt.Fprintf(out, "CPU(s):         %d\n", vcpus)
			fmt.Fprintf(out, "CPU time:       %.1fs\n", float64(cpuTime)/1e9)
			fmt.Fprintf(out, "Max memory:     %d KiB\n", maxMem)
			fmt.Fprintf(out, "Used memory:    %d KiB\n", mem)
			return nil
		})
	},
}

var dumpxmlInactive bool

var dumpxmlCmd = &cobra.Command{
	Use:   "dumpxml <domain>",
	Short: "Print the domain XML (like virsh dumpxml)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			var flags libvirt.DomainXMLFlags
			if dumpxmlInactive {
				flags |= libvirt.DomainXMLInactive
			}
			xml, err := l.DomainGetXMLDesc(d, flags)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), xml)
			return nil
		})
	},
}

var domidCmd = &cobra.Command{
	Use:   "domid <domain>",
	Short: "Print the domain id",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			id := "-"
			if d.ID > 0 {
				id = fmt.Sprintf("%d", d.ID)
			}
			fmt.Fprintln(cmd.OutOrStdout(), id)
			return nil
		})
	},
}

var domuuidCmd = &cobra.Command{
	Use:   "domuuid <domain>",
	Short: "Print the domain UUID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), virtcli.FormatUUID(d.UUID))
			return nil
		})
	},
}

var domhostnameCmd = &cobra.Command{
	Use:   "domhostname <domain>",
	Short: "Print the guest hostname",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			if h, err := l.DomainGetHostname(d, 0); err == nil && h != "" {
				fmt.Fprintln(cmd.OutOrStdout(), h)
				return nil
			}
			if h := virtcli.GuestHostname(l, d); h != "" {
				fmt.Fprintln(cmd.OutOrStdout(), h)
				return nil
			}
			return fmt.Errorf("hostname unavailable for %s", args[0])
		})
	},
}

var domstateCmd = &cobra.Command{
	Use:   "domstate <domain>",
	Short: "Print the domain state",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			state, _, err := l.DomainGetState(d, 0)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), virtcli.StateName(uint8(state)))
			return nil
		})
	},
}

var vcpucountCmd = &cobra.Command{
	Use:   "vcpucount <domain>",
	Short: "Print the current vCPU count",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			_, _, _, vcpus, _, err := l.DomainGetInfo(d)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), vcpus)
			return nil
		})
	},
}

func init() {
	dumpxmlCmd.Flags().BoolVar(&dumpxmlInactive, "inactive", false, "show inactive (persistent) XML")
	rootCmd.AddCommand(dominfoCmd, dumpxmlCmd, domidCmd, domuuidCmd,
		domhostnameCmd, domstateCmd, vcpucountCmd)
}
