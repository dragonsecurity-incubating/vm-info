package cmd

import (
	"context"
	"fmt"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var qgaTimeout int32

var qemuAgentCmd = &cobra.Command{
	Use:   "qemu-agent-command <domain> <command-json>",
	Short: "Send a QEMU guest-agent command (like virsh qemu-agent-command)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			res, err := p.AgentCommand(ctx, vm, args[1], qgaTimeout)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), res)
			return nil
		})
	},
}

func init() {
	qemuAgentCmd.Flags().Int32Var(&qgaTimeout, "timeout", 5, "agent command timeout (seconds, libvirt only)")
	qemuAgentCmd.Annotations = map[string]string{MutatesAnnotation: "true"}
	rootCmd.AddCommand(qemuAgentCmd)
}
