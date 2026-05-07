package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/dragonsecurity/vm-info/internal/provider"
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Inspect async tasks (Proxmox UPIDs)",
	Long: `Inspect async tasks. Proxmox returns a UPID for long-running operations
(notably 'backup create'); use these subcommands to check or follow them.

libvirt operations are synchronous so this whole subtree returns
"not supported" against qemu:// connections.`,
}

var taskStatusCmd = &cobra.Command{
	Use:   "status <upid>",
	Short: "Print one-shot task status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withProvider(func(ctx context.Context, p provider.Provider) error {
			s, err := p.TaskStatus(ctx, args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "UPID:    %s\n", s.UPID)
			fmt.Fprintf(out, "Type:    %s\n", s.Type)
			fmt.Fprintf(out, "Node:    %s\n", s.Node)
			fmt.Fprintf(out, "User:    %s\n", s.User)
			fmt.Fprintf(out, "ID:      %s\n", s.ID)
			fmt.Fprintf(out, "Started: %s\n", formatTime(s.StartTime))
			fmt.Fprintf(out, "Status:  %s\n", s.Status)
			if !s.Running {
				fmt.Fprintf(out, "Exit:    %s\n", dashIfEmpty(s.ExitStatus))
			}
			return nil
		})
	},
}

var taskLogCmd = &cobra.Command{
	Use:   "log <upid>",
	Short: "Print the task log",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withProvider(func(ctx context.Context, p provider.Provider) error {
			lines, err := p.TaskLog(ctx, args[0], 0)
			if err != nil {
				return err
			}
			for _, l := range lines {
				fmt.Fprintln(cmd.OutOrStdout(), l.Text)
			}
			return nil
		})
	},
}

var taskWatchInterval time.Duration

var taskWatchCmd = &cobra.Command{
	Use:   "watch <upid>",
	Short: "Tail a task's log until it finishes; exit non-zero on task failure",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withProvider(func(ctx context.Context, p provider.Provider) error {
			return watchTask(ctx, cmd, p, args[0])
		})
	},
}

func watchTask(ctx context.Context, cmd *cobra.Command, p provider.Provider, upid string) error {
	out := cmd.OutOrStdout()
	next := 0
	tick := time.NewTicker(taskWatchInterval)
	defer tick.Stop()

	flush := func() error {
		lines, err := p.TaskLog(ctx, upid, next)
		if err != nil {
			return err
		}
		for _, l := range lines {
			fmt.Fprintln(out, l.Text)
			if l.N >= next {
				next = l.N + 1
			}
		}
		return nil
	}

	for {
		st, err := p.TaskStatus(ctx, upid)
		if err != nil {
			return err
		}
		if err := flush(); err != nil {
			return err
		}
		if !st.Running {
			if st.ExitStatus != "" && st.ExitStatus != "OK" {
				return fmt.Errorf("task %s finished with exit status %q", upid, st.ExitStatus)
			}
			fmt.Fprintf(out, "task %s finished: %s\n", upid, dashIfEmpty(st.ExitStatus))
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func init() {
	taskWatchCmd.Flags().DurationVar(&taskWatchInterval, "interval", 2*time.Second,
		"poll interval while task is running")
	taskCmd.AddCommand(taskStatusCmd, taskLogCmd, taskWatchCmd)
	rootCmd.AddCommand(taskCmd)
}
