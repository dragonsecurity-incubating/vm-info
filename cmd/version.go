package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// These are populated at link time by the Makefile via -ldflags '-X ...'.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "vm-info %s (%s)\n", Version, Commit)
		fmt.Fprintf(out, "  built: %s\n", Date)
		fmt.Fprintf(out, "  go:    %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	rootCmd.Version = fmt.Sprintf("%s (%s)", Version, Commit)
	rootCmd.AddCommand(versionCmd)
}
