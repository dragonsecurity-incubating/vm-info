package cmd

import (
	"fmt"

	"github.com/digitalocean/go-libvirt"
	"github.com/spf13/cobra"
)

var qgaTimeout int32

var qemuAgentCmd = &cobra.Command{
	Use:   "qemu-agent-command <domain> <command-json>",
	Short: "Send a QEMU guest-agent command (like virsh qemu-agent-command)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withLibvirt(func(l *libvirt.Libvirt) error {
			d, err := lookup(l, args[0])
			if err != nil {
				return err
			}
			res, err := l.QEMUDomainAgentCommand(d, args[1], qgaTimeout, 0)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(res) == 0 {
				fmt.Fprintln(out, "")
				return nil
			}
			fmt.Fprintln(out, res[0])
			return nil
		})
	},
}

func init() {
	qemuAgentCmd.Flags().Int32Var(&qgaTimeout, "timeout", 5, "agent command timeout (seconds)")
	qemuAgentCmd.Annotations = map[string]string{MutatesAnnotation: "true"}
	rootCmd.AddCommand(qemuAgentCmd)
}
