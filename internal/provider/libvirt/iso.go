package libvirt

import (
	"context"
	"fmt"
	"strings"

	"github.com/digitalocean/go-libvirt"
	"github.com/dragonsecurity/vm-info/internal/provider"
)

// AttachISO inserts an ISO into a cdrom slot. If the slot already exists
// as a cdrom device, libvirt's update-device path swaps the media;
// otherwise the slot is attached fresh.
func (p *Provider) AttachISO(_ context.Context, vm provider.VM, slot, source string) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	exists, err := p.cdromSlotExists(d, slot)
	if err != nil {
		return err
	}
	xmlBody := cdromXML(slot, busFromSlot(slot), source)
	flags := libvirt.DomainDeviceModifyLive | libvirt.DomainDeviceModifyConfig | libvirt.DomainDeviceModifyForce
	if exists {
		return p.l.DomainUpdateDeviceFlags(d, xmlBody, flags)
	}
	return p.l.DomainAttachDeviceFlags(d, xmlBody, uint32(flags))
}

// EjectISO clears the media from a cdrom slot but leaves the slot itself
// attached. UpdateDevice with an empty <source/> is libvirt's eject path.
func (p *Provider) EjectISO(_ context.Context, vm provider.VM, slot string) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	xmlBody := cdromXML(slot, busFromSlot(slot), "")
	flags := libvirt.DomainDeviceModifyLive | libvirt.DomainDeviceModifyConfig | libvirt.DomainDeviceModifyForce
	return p.l.DomainUpdateDeviceFlags(d, xmlBody, flags)
}

func (p *Provider) cdromSlotExists(d libvirt.Domain, slot string) (bool, error) {
	xmlStr, err := p.l.DomainGetXMLDesc(d, 0)
	if err != nil {
		return false, err
	}
	dx, err := parseDomainXML(xmlStr)
	if err != nil {
		return false, err
	}
	for _, dk := range dx.Devices.Disks {
		if dk.Target.Dev == slot {
			return true, nil
		}
	}
	return false, nil
}

func busFromSlot(slot string) string {
	switch {
	case strings.HasPrefix(slot, "hd"):
		return "ide"
	case strings.HasPrefix(slot, "sd"):
		return "sata"
	case strings.HasPrefix(slot, "vd"):
		return "virtio"
	case strings.HasPrefix(slot, "xvd"):
		return "xen"
	}
	return "ide"
}

func cdromXML(slot, bus, source string) string {
	if source == "" {
		return fmt.Sprintf(`<disk type='file' device='cdrom'>
  <driver name='qemu' type='raw'/>
  <target dev='%s' bus='%s'/>
  <readonly/>
</disk>`, slot, bus)
	}
	return fmt.Sprintf(`<disk type='file' device='cdrom'>
  <driver name='qemu' type='raw'/>
  <source file='%s'/>
  <target dev='%s' bus='%s'/>
  <readonly/>
</disk>`, source, slot, bus)
}
