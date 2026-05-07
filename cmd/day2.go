package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

// --- autostart -----------------------------------------------------------

var autostartCmd = &cobra.Command{
	Use:   "autostart <domain> [on|off]",
	Short: "Show or toggle whether a VM starts at host boot",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Show form is read-only; set form requires --rw. Gate locally
		// rather than via PersistentPreRunE so the read path stays open.
		if len(args) == 2 && !flagReadWrite {
			return fmt.Errorf("%q with a value is mutating; rerun with --rw to allow it",
				cmd.CommandPath())
		}
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if len(args) == 1 {
				on, err := p.GetAutostart(ctx, vm)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), boolOnOff(on))
				return nil
			}
			on, err := parseOnOff(args[1])
			if err != nil {
				return err
			}
			if err := p.SetAutostart(ctx, vm, on); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Autostart for %s set to %s\n", vm.Name, boolOnOff(on))
			return nil
		})
	},
}

// --- reset ---------------------------------------------------------------

var resetCmd = &cobra.Command{
	Use:   "reset <domain>",
	Short: "Hard-reset a VM (immediate, no graceful shutdown)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.Reset(ctx, vm); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain %s reset\n", vm.Name)
			return nil
		})
	},
}

// --- resize --------------------------------------------------------------

var (
	resizeLive   bool
	resizeConfig bool
)

var resizeCmd = &cobra.Command{
	Use:   "resize",
	Short: "Resize a VM's CPUs, memory, or a disk",
}

var resizeCPUCmd = &cobra.Command{
	Use:   "cpu <domain> <count>",
	Short: "Set the vCPU count",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			n, err := strconv.Atoi(args[1])
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid CPU count %q", args[1])
			}
			if err := p.SetVCPUs(ctx, vm, n, provider.ResizeFlags{Live: resizeLive, Config: resizeConfig}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s vCPUs set to %d\n", vm.Name, n)
			return nil
		})
	},
}

var resizeMemoryCmd = &cobra.Command{
	Use:   "memory <domain> <size>",
	Short: "Set the memory allocation (e.g. 4G, 4096M, 4096)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			mib, err := parseSizeMiB(args[1])
			if err != nil {
				return err
			}
			if err := p.SetMemoryMiB(ctx, vm, mib, provider.ResizeFlags{Live: resizeLive, Config: resizeConfig}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s memory set to %d MiB\n", vm.Name, mib)
			return nil
		})
	},
}

var resizeDiskCmd = &cobra.Command{
	Use:   "disk <domain> <target> <size>",
	Short: "Resize a disk to the absolute size (e.g. 32G, 64G)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			bytes, err := parseSizeBytes(args[2])
			if err != nil {
				return err
			}
			if err := p.ResizeDisk(ctx, vm, args[1], bytes); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s disk %s resized to %d bytes (guest must grow the FS)\n",
				vm.Name, args[1], bytes)
			return nil
		})
	},
}

// --- migrate -------------------------------------------------------------

var (
	migrateLive   bool
	migrateOnline bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate <domain> <target>",
	Short: "Migrate a VM to another node (proxmox) or peer libvirtd (libvirt)",
	Long: `Migrate a VM to another host.

  proxmox → target is a cluster node name (e.g. "pve2"); --online for live
            migration of a running guest.
  libvirt → not implemented in vm-info; use 'virsh migrate <vm>
            qemu+ssh://target/system' (optionally --live --persistent).`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args[:1], func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			if err := p.Migrate(ctx, vm, args[1], provider.MigrateOpts{Live: migrateLive, Online: migrateOnline}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s migration to %s started\n", vm.Name, args[1])
			return nil
		})
	},
}

// --- helpers --------------------------------------------------------------

func boolOnOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "yes", "true", "1":
		return true, nil
	case "off", "no", "false", "0":
		return false, nil
	}
	return false, fmt.Errorf("expected on/off, got %q", s)
}

func parseSizeMiB(s string) (uint64, error) {
	b, err := parseSizeBytes(s)
	if err != nil {
		return 0, err
	}
	return b / (1 << 20), nil
}

// parseSizeBytes accepts plain integers (interpreted as MiB to match virsh
// /qm conventions when used with memory) and suffixed forms like 32G, 4096M,
// 1T, 512K. Decimal values are supported.
func parseSizeBytes(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	mult := uint64(1 << 20) // bare numbers default to MiB
	last := s[len(s)-1]
	switch last {
	case 'K', 'k':
		mult = 1 << 10
		s = s[:len(s)-1]
	case 'M', 'm':
		mult = 1 << 20
		s = s[:len(s)-1]
	case 'G', 'g':
		mult = 1 << 30
		s = s[:len(s)-1]
	case 'T', 't':
		mult = 1 << 40
		s = s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return uint64(v * float64(mult)), nil
}

func init() {
	resizeCmd.PersistentFlags().BoolVar(&resizeLive, "live", false, "apply to the running VM (libvirt only — proxmox always live when supported)")
	resizeCmd.PersistentFlags().BoolVar(&resizeConfig, "config", false, "persist across reboots (libvirt only — proxmox config is always persistent)")
	resizeCmd.AddCommand(resizeCPUCmd, resizeMemoryCmd, resizeDiskCmd)

	migrateCmd.Flags().BoolVar(&migrateLive, "live", false, "live migration (no pause) when the backend supports it")
	migrateCmd.Flags().BoolVar(&migrateOnline, "online", false, "proxmox: online migration of a running guest")

	mutating := []*cobra.Command{
		resetCmd,
		resizeCPUCmd, resizeMemoryCmd, resizeDiskCmd,
		migrateCmd,
	}
	for _, c := range mutating {
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations[MutatesAnnotation] = "true"
	}

	rootCmd.AddCommand(autostartCmd, resetCmd, resizeCmd, migrateCmd)
}
