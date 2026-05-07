package virtcli

import (
	"sort"
	"strings"

	"github.com/digitalocean/go-libvirt"
)

// VMInfo is the per-VM data shown in the default vm-info table.
type VMInfo struct {
	Name      string
	ID        string
	State     string
	Running   bool
	VCPUs     int
	RAMMiB    uint64
	Hostname  string
	IPv4s     []string
	MACs      []string
	XML       *DomainXML
	RawDomain libvirt.Domain
}

// ListAllDomains returns every defined domain, running or not, sorted by name.
func ListAllDomains(l *libvirt.Libvirt) ([]libvirt.Domain, error) {
	doms, _, err := l.ConnectListAllDomains(1024,
		libvirt.ConnectListDomainsActive|libvirt.ConnectListDomainsInactive)
	if err != nil {
		return nil, err
	}
	sort.Slice(doms, func(i, j int) bool { return doms[i].Name < doms[j].Name })
	return doms, nil
}

// CollectVMInfo populates a VMInfo for one domain, applying the supplied
// CIDR filter to IPv4 addresses. Errors from individual sub-calls are
// swallowed so a single misbehaving guest can't blank the whole table.
func CollectVMInfo(l *libvirt.Libvirt, d libvirt.Domain, filter *CIDRFilter) VMInfo {
	v := VMInfo{Name: d.Name, RawDomain: d, ID: "-"}
	if d.ID > 0 {
		v.ID = itoa(int(d.ID))
	}

	state, maxMem, mem, vcpus, _, err := l.DomainGetInfo(d)
	if err == nil {
		v.State = StateName(state)
		v.Running = IsRunning(state)
		v.VCPUs = int(vcpus)
		ram := mem
		if ram == 0 {
			ram = maxMem
		}
		v.RAMMiB = ram / 1024
	} else {
		v.State = "-"
	}

	if xmlStr, err := l.DomainGetXMLDesc(d, 0); err == nil {
		if dx, err := ParseDomainXML(xmlStr); err == nil {
			v.XML = dx
			for _, iface := range dx.Devices.Interfaces {
				if iface.MAC.Address != "" {
					v.MACs = append(v.MACs, iface.MAC.Address)
				}
			}
		}
	}

	v.Hostname = lookupHostname(l, d)
	v.IPv4s = lookupIPv4s(l, d, v.Running, filter)

	if v.Hostname == "" {
		v.Hostname = "-"
	}
	if len(v.MACs) == 0 {
		v.MACs = []string{"-"}
	}
	if len(v.IPv4s) == 0 {
		v.IPv4s = []string{"-"}
	}
	return v
}

func lookupHostname(l *libvirt.Libvirt, d libvirt.Domain) string {
	if h, err := l.DomainGetHostname(d, 0); err == nil && h != "" {
		return h
	}
	return GuestHostname(l, d)
}

func lookupIPv4s(l *libvirt.Libvirt, d libvirt.Domain, running bool, filter *CIDRFilter) []string {
	if !running {
		return nil
	}
	seen := map[string]struct{}{}
	add := func(ip string) {
		if ip == "" {
			return
		}
		if filter != nil && !filter.Allow(ip) {
			return
		}
		if !looksLikeIPv4(ip) {
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
		if ifs, err := GuestInterfaces(l, d); err == nil {
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

// FormatIPv4Column renders a list of IPv4 addresses for the table column,
// collapsing to "first (+N)" unless wide mode is set.
func FormatIPv4Column(ips []string, wide bool) string {
	if len(ips) == 0 || (len(ips) == 1 && ips[0] == "-") {
		return "-"
	}
	if wide || len(ips) == 1 {
		return strings.Join(ips, ",")
	}
	return ips[0] + " (+" + itoa(len(ips)-1) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
