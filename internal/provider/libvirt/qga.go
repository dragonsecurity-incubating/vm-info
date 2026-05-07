package libvirt

import (
	"encoding/json"

	"github.com/digitalocean/go-libvirt"
)

type qgaResponse struct {
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

// guestHostname asks the running QEMU guest agent for the hostname.
func guestHostname(l *libvirt.Libvirt, dom libvirt.Domain) string {
	if raw, err := qga(l, dom, `{"execute":"guest-get-host-name"}`, 2); err == nil && len(raw) > 0 {
		var r qgaResponse
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
	if raw, err := qga(l, dom, `{"execute":"guest-info"}`, 2); err == nil && len(raw) > 0 {
		var r qgaResponse
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

type qgaIface struct {
	Name        string         `json:"name"`
	HardwareMAC string         `json:"hardware-address"`
	IPAddresses []qgaIfaceAddr `json:"ip-addresses"`
}

type qgaIfaceAddr struct {
	Type   string `json:"ip-address-type"`
	Addr   string `json:"ip-address"`
	Prefix int    `json:"prefix"`
}

func guestInterfaces(l *libvirt.Libvirt, dom libvirt.Domain) ([]qgaIface, error) {
	raw, err := qga(l, dom, `{"execute":"guest-network-get-interfaces"}`, 2)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var r qgaResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, nil
	}
	var ifs []qgaIface
	if err := json.Unmarshal(r.Return, &ifs); err != nil {
		return nil, err
	}
	return ifs, nil
}
