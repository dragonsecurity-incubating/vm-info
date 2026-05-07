package virtcli

import "github.com/digitalocean/go-libvirt"

// StateName returns the human label virsh prints for a libvirt domain state.
func StateName(s uint8) string {
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

// IsRunning reports whether the state value represents a running guest.
func IsRunning(s uint8) bool {
	return libvirt.DomainState(s) == libvirt.DomainRunning
}

// FormatUUID renders a 16-byte libvirt UUID as the canonical 8-4-4-4-12 form.
func FormatUUID(u libvirt.UUID) string {
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
