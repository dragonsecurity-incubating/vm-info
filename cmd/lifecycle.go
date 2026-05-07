package cmd

import (
	"fmt"

	"github.com/digitalocean/go-libvirt"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start <domain>",
	Short: "Start a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			if err := l.DomainCreate(d); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s started\n", d.Name)
			return nil
		})
	},
}

var shutdownCmd = &cobra.Command{
	Use:   "shutdown <domain>",
	Short: "Gracefully shutdown a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			if err := l.DomainShutdown(d); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s is being shutdown\n", d.Name)
			return nil
		})
	},
}

var destroyCmd = &cobra.Command{
	Use:   "destroy <domain>",
	Short: "Forcefully stop a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			if err := l.DomainDestroy(d); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s destroyed\n", d.Name)
			return nil
		})
	},
}

var rebootAcpi bool

var rebootCmd = &cobra.Command{
	Use:   "reboot <domain>",
	Short: "Reboot a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			flags := libvirt.DomainRebootDefault
			if rebootAcpi {
				flags = libvirt.DomainRebootAcpiPowerBtn
			}
			if err := l.DomainReboot(d, flags); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s is being rebooted\n", d.Name)
			return nil
		})
	},
}

var suspendCmd = &cobra.Command{
	Use:   "suspend <domain>",
	Short: "Suspend a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			if err := l.DomainSuspend(d); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s suspended\n", d.Name)
			return nil
		})
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume <domain>",
	Short: "Resume a suspended domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			if err := l.DomainResume(d); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s resumed\n", d.Name)
			return nil
		})
	},
}

func init() {
	rebootCmd.Flags().BoolVar(&rebootAcpi, "acpi", false, "use ACPI power button instead of default")
	mutating := []*cobra.Command{startCmd, shutdownCmd, destroyCmd, rebootCmd, suspendCmd, resumeCmd}
	for _, c := range mutating {
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations[MutatesAnnotation] = "true"
	}
	rootCmd.AddCommand(mutating...)
}
