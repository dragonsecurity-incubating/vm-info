// Package libvirt implements provider.Provider on top of digitalocean/go-libvirt.
package libvirt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/digitalocean/go-libvirt"
	"github.com/dragonsecurity/vm-info/internal/provider"
)

// Provider wraps a *libvirt.Libvirt as a provider.Provider.
type Provider struct {
	l   *libvirt.Libvirt
	uri string
}

func init() {
	for _, scheme := range []string{
		"qemu", "qemu+ssh", "qemu+tcp", "qemu+tls", "qemu+libssh", "qemu+libssh2",
	} {
		provider.Register(scheme, Connect)
	}
}

// Connect dials libvirt at the given URI.
func Connect(uri string) (provider.Provider, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse uri %q: %w", uri, err)
	}
	l, err := libvirt.ConnectToURI(u)
	if err != nil {
		return nil, err
	}
	return &Provider{l: l, uri: uri}, nil
}

func (p *Provider) Close() error { return p.l.Disconnect() }
func (p *Provider) Kind() string { return "libvirt" }
func (p *Provider) URI() string  { return p.uri }

func (p *Provider) toVM(d libvirt.Domain) provider.VM {
	id := "-"
	if d.ID > 0 {
		id = fmt.Sprintf("%d", d.ID)
	}
	return provider.VM{
		Name:     d.Name,
		ID:       id,
		UUID:     formatUUID(d.UUID),
		Provider: "libvirt",
	}
}

func (p *Provider) lookupDomain(name string) (libvirt.Domain, error) {
	return p.l.DomainLookupByName(name)
}

func (p *Provider) List(_ context.Context) ([]provider.VM, error) {
	doms, _, err := p.l.ConnectListAllDomains(1024,
		libvirt.ConnectListDomainsActive|libvirt.ConnectListDomainsInactive)
	if err != nil {
		return nil, err
	}
	sort.Slice(doms, func(i, j int) bool { return doms[i].Name < doms[j].Name })
	out := make([]provider.VM, 0, len(doms))
	for _, d := range doms {
		out = append(out, p.toVM(d))
	}
	return out, nil
}

func (p *Provider) Lookup(_ context.Context, name string) (provider.VM, error) {
	d, err := p.lookupDomain(name)
	if err != nil {
		return provider.VM{}, err
	}
	return p.toVM(d), nil
}

func (p *Provider) Info(_ context.Context, vm provider.VM, filter *provider.CIDRFilter) (provider.Info, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return provider.Info{}, err
	}
	info := provider.Info{VM: p.toVM(d)}

	state, maxMem, mem, vcpus, _, err := p.l.DomainGetInfo(d)
	if err == nil {
		info.State = stateName(state)
		info.Running = libvirt.DomainState(state) == libvirt.DomainRunning
		info.VCPUs = int(vcpus)
		ram := mem
		if ram == 0 {
			ram = maxMem
		}
		info.RAMMiB = ram / 1024
		info.MaxMiB = maxMem / 1024
	} else {
		info.State = "-"
	}

	xmlStr, xmlErr := p.l.DomainGetXMLDesc(d, 0)
	if xmlErr == nil {
		if dx, err := parseDomainXML(xmlStr); err == nil {
			for _, iface := range dx.Devices.Interfaces {
				if iface.MAC.Address != "" {
					info.MACs = append(info.MACs, iface.MAC.Address)
				}
			}
		}
	}

	if h, err := p.l.DomainGetHostname(d, 0); err == nil && h != "" {
		info.Hostname = h
	} else if h := guestHostname(p.l, d); h != "" {
		info.Hostname = h
	}

	if info.Running {
		info.IPv4s = collectIPv4s(p.l, d, filter)
	}
	return info, nil
}

func (p *Provider) State(_ context.Context, vm provider.VM) (string, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return "", err
	}
	state, _, err := p.l.DomainGetState(d, 0)
	if err != nil {
		return "", err
	}
	return stateName(uint8(state)), nil
}

func (p *Provider) Hostname(_ context.Context, vm provider.VM) (string, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return "", err
	}
	if h, err := p.l.DomainGetHostname(d, 0); err == nil && h != "" {
		return h, nil
	}
	if h := guestHostname(p.l, d); h != "" {
		return h, nil
	}
	return "", fmt.Errorf("hostname unavailable for %s", vm.Name)
}

func (p *Provider) Interfaces(_ context.Context, vm provider.VM, source string) ([]provider.Interface, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return nil, err
	}
	var src libvirt.DomainInterfaceAddressesSource
	switch source {
	case "lease", "":
		src = libvirt.DomainInterfaceAddressesSrcLease
	case "agent":
		src = libvirt.DomainInterfaceAddressesSrcAgent
	case "arp":
		src = libvirt.DomainInterfaceAddressesSrcArp
	default:
		return nil, fmt.Errorf("source must be one of: lease, agent, arp")
	}
	ifs, err := p.l.DomainInterfaceAddresses(d, uint32(src), 0)
	if err != nil {
		return nil, err
	}
	out := []provider.Interface{}
	for _, iface := range ifs {
		mac := ""
		if len(iface.Hwaddr) > 0 {
			mac = iface.Hwaddr[0]
		}
		if len(iface.Addrs) == 0 {
			out = append(out, provider.Interface{Name: iface.Name, MAC: mac})
			continue
		}
		for _, a := range iface.Addrs {
			proto := "ipv4"
			if libvirt.IPAddrType(a.Type) == libvirt.IPAddrTypeIpv6 {
				proto = "ipv6"
			}
			out = append(out, provider.Interface{
				Name: iface.Name, MAC: mac, Protocol: proto,
				Addr: a.Addr, Prefix: a.Prefix,
			})
		}
	}
	return out, nil
}

func (p *Provider) NetDevices(_ context.Context, vm provider.VM) ([]provider.NetDevice, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return nil, err
	}
	xmlStr, err := p.l.DomainGetXMLDesc(d, 0)
	if err != nil {
		return nil, err
	}
	dx, err := parseDomainXML(xmlStr)
	if err != nil {
		return nil, err
	}
	out := make([]provider.NetDevice, 0, len(dx.Devices.Interfaces))
	for _, iface := range dx.Devices.Interfaces {
		out = append(out, provider.NetDevice{
			Target: iface.Target.Dev,
			Type:   iface.Type,
			Source: iface.SourceLabel(),
			Model:  iface.Model.Type,
			MAC:    iface.MAC.Address,
		})
	}
	return out, nil
}

func (p *Provider) Disks(_ context.Context, vm provider.VM) ([]provider.Disk, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return nil, err
	}
	xmlStr, err := p.l.DomainGetXMLDesc(d, 0)
	if err != nil {
		return nil, err
	}
	dx, err := parseDomainXML(xmlStr)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Disk, 0, len(dx.Devices.Disks))
	for _, dk := range dx.Devices.Disks {
		disk := provider.Disk{
			Target: dk.Target.Dev,
			Bus:    dk.Target.Bus,
			Source: dk.SourcePath(),
			Type:   dk.Type,
			Device: dk.Device,
		}
		if dk.Target.Dev != "" && dk.Device == "disk" {
			if a, c, _, err := p.l.DomainGetBlockInfo(d, dk.Target.Dev, 0); err == nil {
				disk.Capacity = c
				disk.Allocation = a
			}
		}
		out = append(out, disk)
	}
	return out, nil
}

func (p *Provider) BlockInfo(_ context.Context, vm provider.VM, target string) (provider.BlockInfo, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return provider.BlockInfo{}, err
	}
	a, c, ph, err := p.l.DomainGetBlockInfo(d, target, 0)
	if err != nil {
		return provider.BlockInfo{}, err
	}
	return provider.BlockInfo{Capacity: c, Allocation: a, Physical: ph}, nil
}

func (p *Provider) Config(_ context.Context, vm provider.VM, inactive bool) (string, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return "", err
	}
	var flags libvirt.DomainXMLFlags
	if inactive {
		flags |= libvirt.DomainXMLInactive
	}
	return p.l.DomainGetXMLDesc(d, flags)
}

func (p *Provider) Start(_ context.Context, vm provider.VM) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	return p.l.DomainCreate(d)
}

func (p *Provider) Shutdown(_ context.Context, vm provider.VM) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	return p.l.DomainShutdown(d)
}

func (p *Provider) Destroy(_ context.Context, vm provider.VM) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	return p.l.DomainDestroy(d)
}

func (p *Provider) Reboot(_ context.Context, vm provider.VM, opts provider.RebootOpts) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	flags := libvirt.DomainRebootDefault
	if opts.ACPI {
		flags = libvirt.DomainRebootAcpiPowerBtn
	}
	return p.l.DomainReboot(d, flags)
}

func (p *Provider) Suspend(_ context.Context, vm provider.VM) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	return p.l.DomainSuspend(d)
}

func (p *Provider) Resume(_ context.Context, vm provider.VM) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	return p.l.DomainResume(d)
}

func (p *Provider) AgentCommand(_ context.Context, vm provider.VM, cmd string, timeoutSec int32) (string, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return "", err
	}
	res, err := p.l.QEMUDomainAgentCommand(d, cmd, timeoutSec, 0)
	if err != nil {
		return "", err
	}
	if len(res) == 0 {
		return "", nil
	}
	return res[0], nil
}

// --- helpers --------------------------------------------------------------

func stateName(s uint8) string {
	switch libvirt.DomainState(s) {
	case libvirt.DomainNostate:
		return "no state"
	case libvirt.DomainRunning:
		return "running"
	case libvirt.DomainBlocked:
		return "idle"
	case libvirt.DomainPaused:
		return "paused"
	case libvirt.DomainShutdown:
		return "in shutdown"
	case libvirt.DomainShutoff:
		return "shut off"
	case libvirt.DomainCrashed:
		return "crashed"
	case libvirt.DomainPmsuspended:
		return "pmsuspended"
	default:
		return "unknown"
	}
}

func formatUUID(u libvirt.UUID) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	j := 0
	for i, b := range u {
		out[j] = hex[b>>4]
		out[j+1] = hex[b&0x0f]
		j += 2
		if i == 3 || i == 5 || i == 7 || i == 9 {
			out[j] = '-'
			j++
		}
	}
	return string(out)
}

func collectIPv4s(l *libvirt.Libvirt, d libvirt.Domain, filter *provider.CIDRFilter) []string {
	seen := map[string]struct{}{}
	add := func(ip string) {
		if ip == "" || !looksLikeIPv4(ip) {
			return
		}
		if !filter.Allow(ip) {
			return
		}
		seen[ip] = struct{}{}
	}

	if ifs, err := l.DomainInterfaceAddresses(d,
		uint32(libvirt.DomainInterfaceAddressesSrcAgent), 0); err == nil {
		for _, iface := range ifs {
			for _, a := range iface.Addrs {
				if libvirt.IPAddrType(a.Type) == libvirt.IPAddrTypeIpv4 {
					add(a.Addr)
				}
			}
		}
	}
	if len(seen) == 0 {
		if ifs, err := l.DomainInterfaceAddresses(d,
			uint32(libvirt.DomainInterfaceAddressesSrcLease), 0); err == nil {
			for _, iface := range ifs {
				for _, a := range iface.Addrs {
					if libvirt.IPAddrType(a.Type) == libvirt.IPAddrTypeIpv4 {
						add(a.Addr)
					}
				}
			}
		}
	}
	if len(seen) == 0 {
		if ifs, err := guestInterfaces(l, d); err == nil {
			for _, iface := range ifs {
				if iface.Name == "lo" || iface.Name == "lo0" {
					continue
				}
				for _, a := range iface.IPAddresses {
					if a.Type == "ipv4" {
						add(a.Addr)
					}
				}
			}
		}
	}

	out := make([]string, 0, len(seen))
	for ip := range seen {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func looksLikeIPv4(s string) bool {
	dots := 0
	for _, r := range s {
		switch {
		case r == '.':
			dots++
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return dots == 3
}

// --- domain XML projection -----------------------------------------------

type domainXML struct {
	XMLName xml.Name      `xml:"domain"`
	Devices domainDevices `xml:"devices"`
}

type domainDevices struct {
	Disks      []domainDisk      `xml:"disk"`
	Interfaces []domainInterface `xml:"interface"`
}

type domainDisk struct {
	Type   string           `xml:"type,attr"`
	Device string           `xml:"device,attr"`
	Source domainDiskSource `xml:"source"`
	Target domainDiskTarget `xml:"target"`
}

type domainDiskSource struct {
	File   string `xml:"file,attr"`
	Dev    string `xml:"dev,attr"`
	Pool   string `xml:"pool,attr"`
	Volume string `xml:"volume,attr"`
	Name   string `xml:"name,attr"`
}

type domainDiskTarget struct {
	Dev string `xml:"dev,attr"`
	Bus string `xml:"bus,attr"`
}

func (d domainDisk) SourcePath() string {
	switch {
	case d.Source.File != "":
		return d.Source.File
	case d.Source.Dev != "":
		return d.Source.Dev
	case d.Source.Pool != "" && d.Source.Volume != "":
		return d.Source.Pool + "/" + d.Source.Volume
	case d.Source.Name != "":
		return d.Source.Name
	}
	return ""
}

type domainInterface struct {
	Type   string                `xml:"type,attr"`
	MAC    domainInterfaceMAC    `xml:"mac"`
	Source domainInterfaceSource `xml:"source"`
	Model  domainInterfaceModel  `xml:"model"`
	Target domainInterfaceTarget `xml:"target"`
}

type domainInterfaceMAC struct {
	Address string `xml:"address,attr"`
}

type domainInterfaceSource struct {
	Network string `xml:"network,attr"`
	Bridge  string `xml:"bridge,attr"`
	Dev     string `xml:"dev,attr"`
}

type domainInterfaceModel struct {
	Type string `xml:"type,attr"`
}

type domainInterfaceTarget struct {
	Dev string `xml:"dev,attr"`
}

func (i domainInterface) SourceLabel() string {
	switch i.Type {
	case "network":
		if i.Source.Network != "" {
			return "network=" + i.Source.Network
		}
	case "bridge":
		if i.Source.Bridge != "" {
			return "bridge=" + i.Source.Bridge
		}
	case "direct":
		if i.Source.Dev != "" {
			return "direct=" + i.Source.Dev
		}
	}
	for _, s := range []string{i.Source.Network, i.Source.Bridge, i.Source.Dev} {
		if s != "" {
			return s
		}
	}
	return "-"
}

func parseDomainXML(s string) (*domainXML, error) {
	var d domainXML
	if err := xml.Unmarshal([]byte(s), &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// silence unused-import warnings on edge cases
var _ = strings.Join
