package proxmox

// resource is one entry in /cluster/resources?type=vm.
type resource struct {
	ID     string  `json:"id"`     // "qemu/100"
	Type   string  `json:"type"`   // "qemu" or "lxc"
	Name   string  `json:"name"`   // VM name, may be empty for newly-created
	VMID   int     `json:"vmid"`   // numeric VM id
	Node   string  `json:"node"`   // cluster node name
	Status string  `json:"status"` // "running", "stopped", ...
	MaxCPU float64 `json:"maxcpu"`
	MaxMem uint64  `json:"maxmem"` // bytes
	Mem    uint64  `json:"mem"`    // bytes (current)
	Uptime uint64  `json:"uptime"`
}

// status is the body of /nodes/{node}/qemu/{vmid}/status/current.
type status struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	VMID    int    `json:"vmid"`
	CPUs    int    `json:"cpus"`
	MaxMem  uint64 `json:"maxmem"`
	Mem     uint64 `json:"mem"`
	QMPStat string `json:"qmpstatus"`
	PID     int    `json:"pid"`
	Agent   any    `json:"agent"`
}

// agentResponse wraps the JSON returned by /agent/* passthrough endpoints.
type agentResponse struct {
	Result any    `json:"result"`
	Error  string `json:"error"`
}

// agentIfacesResponse is /agent/network-get-interfaces.
type agentIfacesResponse struct {
	Result []agentIface `json:"result"`
}

type agentIface struct {
	Name        string          `json:"name"`
	HardwareMAC string          `json:"hardware-address"`
	IPAddresses []agentIPAddr   `json:"ip-addresses"`
	Statistics  agentIfaceStats `json:"statistics"`
}

type agentIPAddr struct {
	Type   string `json:"ip-address-type"`
	Addr   string `json:"ip-address"`
	Prefix int    `json:"prefix"`
}

type agentIfaceStats struct {
	RxBytes   uint64 `json:"rx-bytes"`
	TxBytes   uint64 `json:"tx-bytes"`
	RxPackets uint64 `json:"rx-packets"`
	TxPackets uint64 `json:"tx-packets"`
}

// agentHostname is /agent/get-host-name.
type agentHostname struct {
	Result struct {
		HostName string `json:"host-name"`
	} `json:"result"`
}
