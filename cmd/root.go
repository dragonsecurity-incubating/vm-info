package cmd

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/digitalocean/go-libvirt"
	"github.com/dragonsecurity/vm-info/internal/virtcli"
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
// Such commands refuse to run unless --rw is supplied.
const MutatesAnnotation = "vm-info.mutates"

var rootCmd = &cobra.Command{
	Use:   "vm-info",
	Short: "Pretty libvirt VM summary and a virsh-compatible CLI",
	Long: `vm-info renders a one-line-per-VM summary of every libvirt domain on the
host, with optional disk details. It also exposes a set of virsh-compatible
subcommands (list, dominfo, domifaddr, start, shutdown, ...) so it can stand
in as a virsh replacement for everyday tasks.

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
		"libvirt connection URI (default $LIBVIRT_DEFAULT_URI or qemu:///system)")
	rootCmd.PersistentFlags().BoolVar(&flagReadWrite, "rw", false,
		"allow mutating operations (vm-info is read-only by default)")
	rootCmd.Flags().BoolVar(&flagShowDisks, "disks", false, "show disks for each VM")
	rootCmd.Flags().BoolVar(&flagWide, "wide", false, "show all IPv4 addresses (no truncation)")
	rootCmd.Flags().StringArrayVar(&flagFilterCIDRs, "filter-cidr", nil,
		"hide IPs inside this CIDR (repeatable)")
}

func withLibvirt(fn func(*libvirt.Libvirt) error) error {
	l, err := virtcli.Connect(flagURI)
	if err != nil {
		return err
	}
	defer func() { _ = l.Disconnect() }()
	return fn(l)
}

func runDefault(cmd *cobra.Command, _ []string) error {
	filter, err := virtcli.NewCIDRFilter(flagFilterCIDRs)
	if err != nil {
		return err
	}
	return withLibvirt(func(l *libvirt.Libvirt) error {
		doms, err := virtcli.ListAllDomains(l)
		if err != nil {
			return err
		}
		printTable(cmd.OutOrStdout(), l, doms, filter, flagShowDisks, flagWide)
		return nil
	})
}

func printTable(w io.Writer, l *libvirt.Libvirt, doms []libvirt.Domain,
	filter *virtcli.CIDRFilter, showDisks, wide bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tID\tSTATE\tvCPU\tRAM(MiB)\tHOSTNAME\tIPv4\tMAC(s)")
	fmt.Fprintln(tw, "----\t--\t-----\t----\t--------\t--------\t----\t------")
	for _, d := range doms {
		v := virtcli.CollectVMInfo(l, d, filter)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\t%s\t%s\n",
			v.Name, v.ID, v.State, v.VCPUs, v.RAMMiB,
			v.Hostname,
			virtcli.FormatIPv4Column(v.IPv4s, wide),
			joinList(v.MACs))
	}
	_ = tw.Flush()

	if showDisks {
		for _, d := range doms {
			v := virtcli.CollectVMInfo(l, d, filter)
			fmt.Fprintf(w, "\n%s disks:\n", v.Name)
			printDisks(w, l, v)
		}
	}
}

func printDisks(w io.Writer, l *libvirt.Libvirt, v virtcli.VMInfo) {
	if v.XML == nil || len(v.XML.Devices.Disks) == 0 {
		fmt.Fprintln(w, "  (no disks)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  TARGET\tBUS\tSOURCE\tCAPACITY\tALLOCATED")
	for _, d := range v.XML.Devices.Disks {
		if d.Device != "disk" {
			continue
		}
		cap, alloc := "?", "?"
		if d.Target.Dev != "" {
			if a, c, _, err := l.DomainGetBlockInfo(v.RawDomain, d.Target.Dev, 0); err == nil {
				cap = formatGiB(c)
				alloc = formatGiB(a)
			}
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n",
			dash(d.Target.Dev), dash(d.Target.Bus), dash(d.SourcePath()), cap, alloc)
	}
	_ = tw.Flush()
}

func formatGiB(b uint64) string {
	const gib = 1024 * 1024 * 1024
	return fmt.Sprintf("%.1fGiB", float64(b)/gib)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func joinList(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	out := s[0]
	for _, v := range s[1:] {
		out += "," + v
	}
	return out
}
