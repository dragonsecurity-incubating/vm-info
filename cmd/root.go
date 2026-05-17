package cmd

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var (
	flagURI         string
	flagShowDisks   bool
	flagWide        bool
	flagFilterCIDRs []string
	flagReadWrite   bool
)

// MutatesAnnotation marks a cobra command as one that changes guest state.
const MutatesAnnotation = "vm-info.mutates"

var rootCmd = &cobra.Command{
	Use:   "vm-info",
	Short: "Pretty VM summary and a virsh-compatible CLI for libvirt and Proxmox VE",
	Long: `vm-info renders a one-line-per-VM summary of every domain on the host
or Proxmox cluster, with optional disk details. It also exposes a set of
virsh-compatible subcommands (list, dominfo, domifaddr, start, shutdown, …)
so it can stand in as a virsh replacement for everyday tasks.

Backends are selected by the --connect URI scheme:

  qemu:///system, qemu+ssh://..., qemu+tcp://...   → libvirt
  pve://host[:8006]/?token=..., proxmox://...      → Proxmox VE

Run with no subcommand to print the summary table.

vm-info is read-only by default for safety: subcommands that change guest
state (start, shutdown, destroy, reboot, suspend, resume, qemu-agent-command)
refuse to run unless --rw is also passed.`,
	SilenceUsage:      true,
	PersistentPreRunE: requireRWIfMutating,
	RunE:              runDefault,
}

func requireRWIfMutating(cmd *cobra.Command, _ []string) error {
	if cmd.Annotations[MutatesAnnotation] != "true" {
		return nil
	}
	if flagReadWrite {
		return nil
	}
	return fmt.Errorf("%q is a mutating operation; rerun with --rw to allow it",
		cmd.CommandPath())
}

// Execute runs the root command.
func Execute() error { return rootCmd.Execute() }

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagURI, "connect", "c", "",
		"connection URI (default $VM_INFO_URI / $LIBVIRT_DEFAULT_URI / qemu:///system)")
	rootCmd.PersistentFlags().BoolVar(&flagReadWrite, "rw", false,
		"allow mutating operations (vm-info is read-only by default)")
	rootCmd.Flags().BoolVar(&flagShowDisks, "disks", false, "show disks for each VM")
	rootCmd.Flags().BoolVar(&flagWide, "wide", false, "show all IPv4 addresses (no truncation)")
	rootCmd.Flags().StringArrayVar(&flagFilterCIDRs, "filter-cidr", nil,
		"hide IPs inside this CIDR (repeatable)")
}

func withProvider(fn func(context.Context, provider.Provider) error) error {
	p, err := provider.Connect(flagURI)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()
	return fn(context.Background(), p)
}

func runDefault(cmd *cobra.Command, _ []string) error {
	filter, err := provider.NewCIDRFilter(flagFilterCIDRs)
	if err != nil {
		return err
	}
	return withProvider(func(ctx context.Context, p provider.Provider) error {
		vms, err := p.List(ctx)
		if err != nil {
			return err
		}
		printTable(ctx, cmd.OutOrStdout(), p, vms, filter, flagShowDisks, flagWide)
		return nil
	})
}

func printTable(ctx context.Context, w io.Writer, p provider.Provider, vms []provider.VM,
	filter *provider.CIDRFilter, showDisks, wide bool) {
	showNode := wide && anyNode(vms)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if showNode {
		fmt.Fprintln(tw, "NAME\tNODE\tID\tSTATE\tvCPU\tRAM(MiB)\tHOSTNAME\tIPv4\tMAC(s)")
		fmt.Fprintln(tw, "----\t----\t--\t-----\t----\t--------\t--------\t----\t------")
	} else {
		fmt.Fprintln(tw, "NAME\tID\tSTATE\tvCPU\tRAM(MiB)\tHOSTNAME\tIPv4\tMAC(s)")
		fmt.Fprintln(tw, "----\t--\t-----\t----\t--------\t--------\t----\t------")
	}
	for _, vm := range vms {
		info, err := p.Info(ctx, vm, filter)
		if err != nil {
			if showNode {
				fmt.Fprintf(tw, "%s\t%s\t-\terror\t-\t-\t-\t-\t-\n", vm.Name, dashIfEmpty(vm.Node))
			} else {
				fmt.Fprintf(tw, "%s\t-\terror\t-\t-\t-\t-\t-\n", vm.Name)
			}
			continue
		}
		if showNode {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\t%s\n",
				info.Name, dashIfEmpty(info.Node), dashIfEmpty(info.ID), info.State, info.VCPUs, info.RAMMiB,
				dashIfEmpty(info.Hostname),
				provider.FormatIPv4(info.IPv4s, wide),
				joinDash(info.MACs))
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\t%s\t%s\n",
				info.Name, dashIfEmpty(info.ID), info.State, info.VCPUs, info.RAMMiB,
				dashIfEmpty(info.Hostname),
				provider.FormatIPv4(info.IPv4s, wide),
				joinDash(info.MACs))
		}
	}
	_ = tw.Flush()

	if showDisks {
		for _, vm := range vms {
			fmt.Fprintf(w, "\n%s disks:\n", vm.Name)
			disks, err := p.Disks(ctx, vm)
			if err != nil {
				fmt.Fprintf(w, "  (error: %v)\n", err)
				continue
			}
			printDisks(w, disks)
		}
	}
}

func printDisks(w io.Writer, disks []provider.Disk) {
	if len(disks) == 0 {
		fmt.Fprintln(w, "  (no disks)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  TARGET\tBUS\tSOURCE\tCAPACITY\tALLOCATED")
	for _, d := range disks {
		if d.Device != "" && d.Device != "disk" {
			continue
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n",
			dashIfEmpty(d.Target), dashIfEmpty(d.Bus), dashIfEmpty(d.Source),
			formatBytes(d.Capacity), formatBytes(d.Allocation))
	}
	_ = tw.Flush()
}

func formatBytes(b uint64) string {
	if b == 0 {
		return "?"
	}
	const gib = 1024 * 1024 * 1024
	return fmt.Sprintf("%.1fGiB", float64(b)/gib)
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func anyNode(vms []provider.VM) bool {
	for _, vm := range vms {
		if vm.Node != "" {
			return true
		}
	}
	return false
}

func joinDash(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	out := s[0]
	for _, v := range s[1:] {
		out += "," + v
	}
	return out
}
