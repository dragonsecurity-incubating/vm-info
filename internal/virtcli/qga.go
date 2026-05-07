package virtcli

import (
	"encoding/json"

	"github.com/digitalocean/go-libvirt"
)

// QGAResponse is the outer shape returned by libvirt's qemu-agent-command
// passthrough (the QGA "return" field has guest-command-specific contents).
type QGAResponse struct {
	Return json.RawMessage `json:"return"`
	Error  *struct {
		Class string `json:"class"`
		Desc  string `json:"desc"`
	} `json:"error"`
}

func qga(l *libvirt.Libvirt, dom libvirt.Domain, cmd string, timeout int32) ([]byte, error) {
	res, err := l.QEMUDomainAgentCommand(dom, cmd, timeout, 0)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, nil
	}
	return []byte(res[0]), nil
}

// GuestHostname asks the running guest agent for the hostname. Returns "" when
// the agent is unreachable or the field is absent — callers should fall back.
func GuestHostname(l *libvirt.Libvirt, dom libvirt.Domain) string {
	raw, err := qga(l, dom, `{"execute":"guest-get-host-name"}`, 2)
	if err == nil && len(raw) > 0 {
		var r QGAResponse
		if json.Unmarshal(raw, &r) == nil && r.Error == nil {
			var asStr string
			if json.Unmarshal(r.Return, &asStr) == nil && asStr != "" {
				return asStr
			}
			var asObj struct {
				HostName string `json:"host-name"`
			}
			if json.Unmarshal(r.Return, &asObj) == nil && asObj.HostName != "" {
				return asObj.HostName
			}
		}
	}
	raw, err = qga(l, dom, `{"execute":"guest-info"}`, 2)
	if err == nil && len(raw) > 0 {
		var r QGAResponse
		if json.Unmarshal(raw, &r) == nil && r.Error == nil {
			var asObj struct {
				Hostname string `json:"hostname"`
			}
			if json.Unmarshal(r.Return, &asObj) == nil && asObj.Hostname != "" {
				return asObj.Hostname
			}
		}
	}
	return ""
}

// QGAInterface mirrors guest-network-get-interfaces output for callers that
// want it as a fallback IP source.
type QGAInterface struct {
	Name        string         `json:"name"`
	HardwareMAC string         `json:"hardware-address"`
	IPAddresses []QGAIPAddress `json:"ip-addresses"`
}

type QGAIPAddress struct {
	Type   string `json:"ip-address-type"`
	Addr   string `json:"ip-address"`
	Prefix int    `json:"prefix"`
}

func GuestInterfaces(l *libvirt.Libvirt, dom libvirt.Domain) ([]QGAInterface, error) {
	raw, err := qga(l, dom, `{"execute":"guest-network-get-interfaces"}`, 2)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var r QGAResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, nil
	}
	var ifs []QGAInterface
	if err := json.Unmarshal(r.Return, &ifs); err != nil {
		return nil, err
	}
	return ifs, nil
}
