package libvirt

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/dragonsecurity/vm-info/internal/provider"
)

// libvirt vCPU and memory mod flags from libvirt-domain.h. Not exposed as
// named constants in go-libvirt yet, so we declare them locally.
const (
	domainVCPULive   uint32 = 1
	domainVCPUConfig uint32 = 2
)

func (p *Provider) Stats(_ context.Context, vm provider.VM) (provider.Stats, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return provider.Stats{}, err
	}
	s := provider.Stats{SampledAt: time.Now()}

	state, maxMem, mem, vcpus, cpuTime, err := p.l.DomainGetInfo(d)
	if err != nil {
		return s, err
	}
	s.VCPUs = int(vcpus)
	s.CPUTimeNanos = cpuTime
	s.MemTotalBytes = maxMem * 1024
	s.MemUsedBytes = mem * 1024

	// Disk and network stats are per-device; sum across devices in the XML.
	if libvirt.DomainState(state) == libvirt.DomainRunning {
		if xmlStr, err := p.l.DomainGetXMLDesc(d, 0); err == nil {
			if dx, err := parseDomainXML(xmlStr); err == nil {
				for _, dk := range dx.Devices.Disks {
					if dk.Target.Dev == "" {
						continue
					}
					_, rdBytes, _, wrBytes, _, err := p.l.DomainBlockStats(d, dk.Target.Dev)
					if err != nil {
						continue
					}
					if rdBytes > 0 {
						s.DiskReadBytes += uint64(rdBytes)
					}
					if wrBytes > 0 {
						s.DiskWriteBytes += uint64(wrBytes)
					}
				}
				for _, iface := range dx.Devices.Interfaces {
					if iface.Target.Dev == "" {
						continue
					}
					rxB, _, _, _, txB, _, _, _, err := p.l.DomainInterfaceStats(d, iface.Target.Dev)
					if err != nil {
						continue
					}
					if rxB > 0 {
						s.NetRXBytes += uint64(rxB)
					}
					if txB > 0 {
						s.NetTXBytes += uint64(txB)
					}
				}
			}
		}
	}
	return s, nil
}

func (p *Provider) GetAutostart(_ context.Context, vm provider.VM) (bool, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return false, err
	}
	v, err := p.l.DomainGetAutostart(d)
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

func (p *Provider) SetAutostart(_ context.Context, vm provider.VM, on bool) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	v := int32(0)
	if on {
		v = 1
	}
	return p.l.DomainSetAutostart(d, v)
}

func (p *Provider) Reset(_ context.Context, vm provider.VM) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	return p.l.DomainReset(d, 0)
}

func resizeFlagsToVCPU(f provider.ResizeFlags) uint32 {
	if !f.Live && !f.Config {
		return 0
	}
	var v uint32
	if f.Live {
		v |= domainVCPULive
	}
	if f.Config {
		v |= domainVCPUConfig
	}
	return v
}

func resizeFlagsToMemory(f provider.ResizeFlags) uint32 {
	var v uint32
	if f.Live {
		v |= uint32(libvirt.DomainMemLive)
	}
	if f.Config {
		v |= uint32(libvirt.DomainMemConfig)
	}
	return v
}

func (p *Provider) SetVCPUs(_ context.Context, vm provider.VM, count int, flags provider.ResizeFlags) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	return p.l.DomainSetVcpusFlags(d, uint32(count), resizeFlagsToVCPU(flags))
}

func (p *Provider) SetMemoryMiB(_ context.Context, vm provider.VM, mib uint64, flags provider.ResizeFlags) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	return p.l.DomainSetMemoryFlags(d, mib*1024, resizeFlagsToMemory(flags))
}

func (p *Provider) ResizeDisk(_ context.Context, vm provider.VM, target string, sizeBytes uint64) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	return p.l.DomainBlockResize(d, target, sizeBytes, libvirt.DomainBlockResizeBytes)
}

func (p *Provider) Migrate(_ context.Context, _ provider.VM, _ string, _ provider.MigrateOpts) error {
	return fmt.Errorf("%w: vm-info doesn't implement the libvirt P2P migration handshake — use 'virsh migrate <vm> qemu+ssh://target/system' (optionally --live --persistent)",
		provider.ErrNotSupported)
}
