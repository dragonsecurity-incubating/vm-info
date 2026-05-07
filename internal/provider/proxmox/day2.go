package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dragonsecurity/vm-info/internal/provider"
)

// pveCurrent extends the basic status with the live counters /status/current
// also returns. Proxmox pre-computes `cpu` as a 0..1 fraction.
type pveCurrent struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	CPU       float64 `json:"cpu"`
	CPUs      int     `json:"cpus"`
	MaxMem    uint64  `json:"maxmem"`
	Mem       uint64  `json:"mem"`
	Balloon   uint64  `json:"balloon"`
	DiskRead  uint64  `json:"diskread"`
	DiskWrite uint64  `json:"diskwrite"`
	NetIn     uint64  `json:"netin"`
	NetOut    uint64  `json:"netout"`
	Uptime    uint64  `json:"uptime"`
}

func (p *Provider) Stats(ctx context.Context, vm provider.VM) (provider.Stats, error) {
	var c pveCurrent
	if err := p.c.get(ctx, p.basePath(vm)+"/status/current", &c); err != nil {
		return provider.Stats{}, err
	}
	return provider.Stats{
		SampledAt:      time.Now(),
		CPUPercent:     c.CPU * 100,
		HasCPUPercent:  true,
		VCPUs:          c.CPUs,
		MemTotalBytes:  c.MaxMem,
		MemUsedBytes:   c.Mem,
		DiskReadBytes:  c.DiskRead,
		DiskWriteBytes: c.DiskWrite,
		NetRXBytes:     c.NetIn,
		NetTXBytes:     c.NetOut,
	}, nil
}

func (p *Provider) GetAutostart(ctx context.Context, vm provider.VM) (bool, error) {
	cfg, err := p.config(ctx, vm)
	if err != nil {
		return false, err
	}
	v, ok := cfg["onboot"]
	if !ok {
		return false, nil
	}
	switch x := v.(type) {
	case bool:
		return x, nil
	case float64:
		return x != 0, nil
	case string:
		return x == "1" || strings.EqualFold(x, "true"), nil
	}
	return false, nil
}

func (p *Provider) SetAutostart(ctx context.Context, vm provider.VM, on bool) error {
	form := url.Values{}
	if on {
		form.Set("onboot", "1")
	} else {
		form.Set("onboot", "0")
	}
	return p.c.do(ctx, "PUT", p.basePath(vm)+"/config", form, nil)
}

func (p *Provider) Reset(ctx context.Context, vm provider.VM) error {
	return p.c.post(ctx, p.basePath(vm)+"/status/reset", nil)
}

func (p *Provider) SetVCPUs(ctx context.Context, vm provider.VM, count int, _ provider.ResizeFlags) error {
	form := url.Values{}
	form.Set("cores", strconv.Itoa(count))
	return p.c.do(ctx, "PUT", p.basePath(vm)+"/config", form, nil)
}

func (p *Provider) SetMemoryMiB(ctx context.Context, vm provider.VM, mib uint64, _ provider.ResizeFlags) error {
	form := url.Values{}
	form.Set("memory", strconv.FormatUint(mib, 10))
	return p.c.do(ctx, "PUT", p.basePath(vm)+"/config", form, nil)
}

// ResizeDisk uses Proxmox's PUT /resize endpoint. Size is converted to a
// human-readable suffix ("32G") to match what `qm resize` accepts.
func (p *Provider) ResizeDisk(ctx context.Context, vm provider.VM, target string, sizeBytes uint64) error {
	form := url.Values{}
	form.Set("disk", target)
	form.Set("size", humanBytes(sizeBytes))
	return p.c.do(ctx, "PUT", p.basePath(vm)+"/resize", form, nil)
}

func (p *Provider) Migrate(ctx context.Context, vm provider.VM, dest string, opts provider.MigrateOpts) error {
	if dest == "" {
		return fmt.Errorf("proxmox: migrate requires a target node name (got empty string)")
	}
	form := url.Values{}
	form.Set("target", dest)
	if opts.Live || opts.Online {
		form.Set("online", "1")
	}
	return p.c.post(ctx, p.basePath(vm)+"/migrate", form)
}

// humanBytes renders bytes as the largest whole-unit suffix Proxmox accepts.
// 1 GiB → "1G", 512 MiB → "512M".
func humanBytes(b uint64) string {
	switch {
	case b >= 1<<30 && b%(1<<30) == 0:
		return strconv.FormatUint(b>>30, 10) + "G"
	case b >= 1<<20 && b%(1<<20) == 0:
		return strconv.FormatUint(b>>20, 10) + "M"
	case b >= 1<<10 && b%(1<<10) == 0:
		return strconv.FormatUint(b>>10, 10) + "K"
	}
	return strconv.FormatUint(b, 10)
}
