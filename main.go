package main

import (
	"fmt"
	"os"

	"github.com/dragonsecurity/vm-info/cmd"

	// Register provider backends. Each blank import runs the provider's
	// init() which registers its URI schemes with internal/provider.
	_ "github.com/dragonsecurity/vm-info/internal/provider/libvirt"
	_ "github.com/dragonsecurity/vm-info/internal/provider/proxmox"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
