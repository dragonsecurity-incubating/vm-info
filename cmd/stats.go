package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats <domain>",
	Short: "Print one-shot resource counters for a single VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithVM(args, func(ctx context.Context, p provider.Provider, vm provider.VM) error {
			s, err := p.Stats(ctx, vm)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Sampled:    %s\n", s.SampledAt.Local().Format(time.RFC3339))
			fmt.Fprintf(out, "vCPUs:      %d\n", s.VCPUs)
			if s.HasCPUPercent {
				fmt.Fprintf(out, "CPU%%:       %.2f\n", s.CPUPercent)
			} else {
				fmt.Fprintf(out, "CPU time:   %.1fs (cumulative)\n", float64(s.CPUTimeNanos)/1e9)
			}
			fmt.Fprintf(out, "Memory:     %s used / %s total\n",
				formatBytes(s.MemUsedBytes), formatBytes(s.MemTotalBytes))
			fmt.Fprintf(out, "Disk:       %s read / %s written (cumulative)\n",
				formatBytes(s.DiskReadBytes), formatBytes(s.DiskWriteBytes))
			fmt.Fprintf(out, "Network:    %s rx / %s tx (cumulative)\n",
				formatBytes(s.NetRXBytes), formatBytes(s.NetTXBytes))
			return nil
		})
	},
}

var (
	topInterval time.Duration
	topOnce     bool
	topSort     string
)

var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Live grid of VM resource counters (like top, for VMs)",
	Long: `Continuously refresh a per-VM resource grid: CPU%, memory, disk I/O, net I/O.

Sorts by CPU descending by default. Press Ctrl-C to exit.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withProvider(func(ctx context.Context, p provider.Provider) error {
			ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return runTop(ctx, cmd.OutOrStdout(), p)
		})
	},
}

type sample struct {
	vm    provider.VM
	at    time.Time
	stats provider.Stats
}

func runTop(ctx context.Context, w io.Writer, p provider.Provider) error {
	prev := map[string]sample{}
	for {
		vms, err := p.List(ctx)
		if err != nil {
			return err
		}
		now := map[string]sample{}
		for _, vm := range vms {
			s, err := p.Stats(ctx, vm)
			if err != nil {
				continue
			}
			now[vm.Name] = sample{vm: vm, at: time.Now(), stats: s}
		}
		if !topOnce {
			fmt.Fprint(w, "\033[H\033[J")
			fmt.Fprintf(w, "vm-info top — %s — %d VMs (Ctrl-C to exit)\n\n",
				time.Now().Format(time.RFC3339), len(now))
		}
		renderTop(w, prev, now)
		if topOnce {
			return nil
		}
		select {
		case <-ctx.Done():
			fmt.Fprintln(w)
			return nil
		case <-time.After(topInterval):
		}
		prev = now
	}
}

type topRow struct {
	vm        provider.VM
	state     string
	vcpus     int
	cpuPct    float64
	memUsed   uint64
	memTotal  uint64
	diskRead  float64
	diskWrite float64
	netRX     float64
	netTX     float64
}

func renderTop(w io.Writer, prev, now map[string]sample) {
	rows := make([]topRow, 0, len(now))
	for name, cur := range now {
		r := topRow{
			vm:       cur.vm,
			vcpus:    cur.stats.VCPUs,
			memUsed:  cur.stats.MemUsedBytes,
			memTotal: cur.stats.MemTotalBytes,
		}
		if cur.stats.HasCPUPercent {
			r.cpuPct = cur.stats.CPUPercent
		} else if old, ok := prev[name]; ok {
			dt := cur.at.Sub(old.at).Seconds()
			if dt > 0 && cur.stats.CPUTimeNanos > old.stats.CPUTimeNanos {
				cpuSec := float64(cur.stats.CPUTimeNanos-old.stats.CPUTimeNanos) / 1e9
				if cur.stats.VCPUs > 0 {
					r.cpuPct = (cpuSec / dt) * 100 / float64(cur.stats.VCPUs)
				}
			}
		}
		if old, ok := prev[name]; ok {
			dt := cur.at.Sub(old.at).Seconds()
			if dt > 0 {
				r.diskRead = bytesPerSec(cur.stats.DiskReadBytes, old.stats.DiskReadBytes, dt)
				r.diskWrite = bytesPerSec(cur.stats.DiskWriteBytes, old.stats.DiskWriteBytes, dt)
				r.netRX = bytesPerSec(cur.stats.NetRXBytes, old.stats.NetRXBytes, dt)
				r.netTX = bytesPerSec(cur.stats.NetTXBytes, old.stats.NetTXBytes, dt)
			}
		}
		rows = append(rows, r)
	}

	switch topSort {
	case "name":
		sort.Slice(rows, func(i, j int) bool { return rows[i].vm.Name < rows[j].vm.Name })
	case "mem":
		sort.Slice(rows, func(i, j int) bool { return rows[i].memUsed > rows[j].memUsed })
	default:
		sort.Slice(rows, func(i, j int) bool { return rows[i].cpuPct > rows[j].cpuPct })
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tCPU%\tvCPU\tMEM\tDISK R/s\tDISK W/s\tNET RX/s\tNET TX/s")
	fmt.Fprintln(tw, "--\t----\t----\t----\t---\t-------\t-------\t-------\t-------")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%5.1f\t%d\t%s/%s\t%s\t%s\t%s\t%s\n",
			dashIfEmpty(r.vm.ID),
			r.vm.Name,
			r.cpuPct,
			r.vcpus,
			formatBytes(r.memUsed), formatBytes(r.memTotal),
			formatRate(r.diskRead),
			formatRate(r.diskWrite),
			formatRate(r.netRX),
			formatRate(r.netTX))
	}
	_ = tw.Flush()
}

func bytesPerSec(curr, prev uint64, dt float64) float64 {
	if curr <= prev || dt <= 0 {
		return 0
	}
	return float64(curr-prev) / dt
}

func formatRate(bps float64) string {
	switch {
	case bps >= 1<<30:
		return fmt.Sprintf("%.1fGB/s", bps/(1<<30))
	case bps >= 1<<20:
		return fmt.Sprintf("%.1fMB/s", bps/(1<<20))
	case bps >= 1<<10:
		return fmt.Sprintf("%.1fkB/s", bps/(1<<10))
	case bps > 0:
		return fmt.Sprintf("%.0fB/s", bps)
	}
	return "-"
}

func init() {
	topCmd.Flags().DurationVar(&topInterval, "interval", 2*time.Second, "refresh interval")
	topCmd.Flags().BoolVar(&topOnce, "once", false, "print one snapshot and exit")
	topCmd.Flags().StringVar(&topSort, "sort", "cpu", "sort by: cpu, mem, name")
	rootCmd.AddCommand(statsCmd, topCmd)
}
