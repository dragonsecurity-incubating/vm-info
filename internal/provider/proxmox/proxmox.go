package proxmox

import (
	"context"
	"crypto/sha1" //nolint:gosec // used only for deriving a UUID-shaped string
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/dragonsecurity/vm-info/internal/provider"
)

// Provider is the Proxmox VE implementation of provider.Provider.
type Provider struct {
	c *client
}

func init() {
	provider.Register("pve", Connect)
	provider.Register("proxmox", Connect)
}

// Connect dials a Proxmox VE cluster. URI form:
//
//	pve://host[:8006]/?token=USER@REALM!TOKENID=SECRET[&insecure=1]
//
// Token can also come from the PVE_API_TOKEN env var; insecure can come from
// PVE_INSECURE=1.
func Connect(uri string) (provider.Provider, error) {
	c, err := newClient(uri)
	if err != nil {
		return nil, err
	}
	// Smoke-test: hit /version.
	var v map[string]any
	if err := c.get(context.Background(), "/version", &v); err != nil {
		return nil, fmt.Errorf("proxmox: /version probe failed: %w", err)
	}
	return &Provider{c: c}, nil
}

func (p *Provider) Close() error { return nil }
func (p *Provider) Kind() string { return "proxmox" }
func (p *Provider) URI() string  { return p.c.uri }

// --- list / lookup --------------------------------------------------------

func (p *Provider) listResources(ctx context.Context) ([]resource, error) {
	// Try /cluster/resources first — it's a single call covering every node.
	// Falls back to per-node enumeration on standalone hosts where Proxmox
	// returns 501 for the entire /cluster/* tree.
	var cluster []resource
	if err := p.c.get(ctx, "/cluster/resources?type=vm", &cluster); err == nil {
		out := cluster[:0]
		for _, r := range cluster {
			if r.Type == "qemu" {
				out = append(out, r)
			}
		}
		return out, nil
	} else if !is501(err) {
		return nil, err
	}

	var nodes []nodeInfo
	if err := p.c.get(ctx, "/nodes", &nodes); err != nil {
		return nil, err
	}
	var out []resource
	for _, n := range nodes {
		var vms []nodeVM
		if err := p.c.get(ctx, "/nodes/"+n.Node+"/qemu", &vms); err != nil {
			return nil, fmt.Errorf("list qemu on node %s: %w", n.Node, err)
		}
		for _, vm := range vms {
			out = append(out, resource{
				ID:     fmt.Sprintf("qemu/%d", vm.VMID),
				Type:   "qemu",
				Name:   vm.Name,
				VMID:   vm.VMID,
				Node:   n.Node,
				Status: vm.Status,
				MaxCPU: float64(vm.CPUs),
				MaxMem: vm.MaxMem,
				Mem:    vm.Mem,
				Uptime: vm.Uptime,
			})
		}
	}
	return out, nil
}

func is501(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ": 501 ")
}

func (p *Provider) toVM(r resource) provider.VM {
	return provider.VM{
		Name:     displayName(r),
		ID:       strconv.Itoa(r.VMID),
		UUID:     deriveUUID(r),
		Node:     r.Node,
		Provider: "proxmox",
	}
}

func displayName(r resource) string {
	if r.Name != "" {
		return r.Name
	}
	return fmt.Sprintf("vm-%d", r.VMID)
}

// deriveUUID gives a stable, UUID-shaped string per VM since the PVE API does
// not surface SMBIOS UUIDs in /cluster/resources. Looks like a UUID for
// display but is content-derived.
func deriveUUID(r resource) string {
	h := sha1.Sum([]byte(fmt.Sprintf("pve:%s:%d", r.Node, r.VMID)))
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	j := 0
	for i := 0; i < 16; i++ {
		out[j] = hex[h[i]>>4]
		out[j+1] = hex[h[i]&0x0f]
		j += 2
		if i == 3 || i == 5 || i == 7 || i == 9 {
			out[j] = '-'
			j++
		}
	}
	return string(out)
}

func (p *Provider) List(ctx context.Context) ([]provider.VM, error) {
	rs, err := p.listResources(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(rs, func(i, j int) bool { return displayName(rs[i]) < displayName(rs[j]) })
	out := make([]provider.VM, 0, len(rs))
	for _, r := range rs {
		out = append(out, p.toVM(r))
	}
	return out, nil
}

func (p *Provider) Lookup(ctx context.Context, name string) (provider.VM, error) {
	rs, err := p.listResources(ctx)
	if err != nil {
		return provider.VM{}, err
	}
	if vmid, err := strconv.Atoi(name); err == nil {
		for _, r := range rs {
			if r.VMID == vmid {
				return p.toVM(r), nil
			}
		}
	}
	for _, r := range rs {
		if displayName(r) == name {
			return p.toVM(r), nil
		}
	}
	return provider.VM{}, fmt.Errorf("proxmox: VM %q not found", name)
}

// --- read methods ---------------------------------------------------------

func (p *Provider) basePath(vm provider.VM) string {
	return fmt.Sprintf("/nodes/%s/qemu/%s", vm.Node, vm.ID)
}

func (p *Provider) Info(ctx context.Context, vm provider.VM, filter *provider.CIDRFilter) (provider.Info, error) {
	info := provider.Info{VM: vm}

	var st status
	if err := p.c.get(ctx, p.basePath(vm)+"/status/current", &st); err != nil {
		return info, err
	}
	info.State = st.Status
	info.Running = st.Status == "running"
	info.VCPUs = st.CPUs
	info.MaxMiB = st.MaxMem / 1024 / 1024
	if st.Mem > 0 {
		info.RAMMiB = st.Mem / 1024 / 1024
	} else {
		info.RAMMiB = info.MaxMiB
	}

	cfg, err := p.config(ctx, vm)
	if err == nil {
		info.MACs = parseConfigMACs(cfg)
	}

	if info.Running {
		if h, err := p.agentHostname(ctx, vm); err == nil && h != "" {
			info.Hostname = h
		}
		info.IPv4s = p.collectIPv4s(ctx, vm, filter)
	}
	return info, nil
}

func (p *Provider) State(ctx context.Context, vm provider.VM) (string, error) {
	var st status
	if err := p.c.get(ctx, p.basePath(vm)+"/status/current", &st); err != nil {
		return "", err
	}
	return st.Status, nil
}

func (p *Provider) Hostname(ctx context.Context, vm provider.VM) (string, error) {
	h, err := p.agentHostname(ctx, vm)
	if err != nil {
		return "", err
	}
	if h == "" {
		return "", fmt.Errorf("hostname unavailable for %s", vm.Name)
	}
	return h, nil
}

func (p *Provider) Interfaces(ctx context.Context, vm provider.VM, source string) ([]provider.Interface, error) {
	if source != "" && source != "agent" {
		return nil, fmt.Errorf("proxmox: only --source agent is supported (got %q)", source)
	}
	ifs, err := p.agentInterfaces(ctx, vm)
	if err != nil {
		return nil, err
	}
	out := []provider.Interface{}
	for _, iface := range ifs {
		mac := iface.HardwareMAC
		if len(iface.IPAddresses) == 0 {
			out = append(out, provider.Interface{Name: iface.Name, MAC: mac})
			continue
		}
		for _, a := range iface.IPAddresses {
			proto := a.Type
			if proto == "" {
				proto = "ipv4"
			}
			out = append(out, provider.Interface{
				Name: iface.Name, MAC: mac, Protocol: proto,
				Addr: a.Addr, Prefix: uint32(a.Prefix),
			})
		}
	}
	return out, nil
}

func (p *Provider) NetDevices(ctx context.Context, vm provider.VM) ([]provider.NetDevice, error) {
	cfg, err := p.config(ctx, vm)
	if err != nil {
		return nil, err
	}
	out := []provider.NetDevice{}
	for k, v := range cfg {
		if !strings.HasPrefix(k, "net") {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		out = append(out, parseNetEntry(k, s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out, nil
}

func (p *Provider) Disks(ctx context.Context, vm provider.VM) ([]provider.Disk, error) {
	cfg, err := p.config(ctx, vm)
	if err != nil {
		return nil, err
	}
	out := []provider.Disk{}
	for k, v := range cfg {
		if !isDiskKey(k) {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		out = append(out, parseDiskEntry(k, s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out, nil
}

func (p *Provider) BlockInfo(ctx context.Context, vm provider.VM, target string) (provider.BlockInfo, error) {
	disks, err := p.Disks(ctx, vm)
	if err != nil {
		return provider.BlockInfo{}, err
	}
	for _, d := range disks {
		if d.Target == target {
			return provider.BlockInfo{
				Capacity:   d.Capacity,
				Allocation: d.Allocation,
				Physical:   d.Capacity,
			}, nil
		}
	}
	return provider.BlockInfo{}, fmt.Errorf("disk %q not found on %s", target, vm.Name)
}

func (p *Provider) Config(ctx context.Context, vm provider.VM, _ bool) (string, error) {
	cfg, err := p.config(ctx, vm)
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s: %v\n", k, cfg[k])
	}
	return sb.String(), nil
}

// --- mutating -------------------------------------------------------------

func (p *Provider) Start(ctx context.Context, vm provider.VM) error {
	return p.c.post(ctx, p.basePath(vm)+"/status/start", nil)
}

func (p *Provider) Shutdown(ctx context.Context, vm provider.VM) error {
	return p.c.post(ctx, p.basePath(vm)+"/status/shutdown", nil)
}

func (p *Provider) Destroy(ctx context.Context, vm provider.VM) error {
	return p.c.post(ctx, p.basePath(vm)+"/status/stop", nil)
}

func (p *Provider) Reboot(ctx context.Context, vm provider.VM, _ provider.RebootOpts) error {
	return p.c.post(ctx, p.basePath(vm)+"/status/reboot", nil)
}

func (p *Provider) Suspend(ctx context.Context, vm provider.VM) error {
	return p.c.post(ctx, p.basePath(vm)+"/status/suspend", nil)
}

func (p *Provider) Resume(ctx context.Context, vm provider.VM) error {
	return p.c.post(ctx, p.basePath(vm)+"/status/resume", nil)
}

// AgentCommand proxies a QGA call. The libvirt CLI takes a JSON envelope
// like {"execute":"guest-info"}; we accept the same form and translate to
// the Proxmox /agent/{cmd} REST shape.
func (p *Provider) AgentCommand(ctx context.Context, vm provider.VM, cmd string, _ int32) (string, error) {
	var env struct {
		Execute   string         `json:"execute"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(cmd), &env); err != nil {
		return "", fmt.Errorf("proxmox: agent command must be JSON like {\"execute\":\"...\"}: %w", err)
	}
	if env.Execute == "" {
		return "", fmt.Errorf("proxmox: missing 'execute' in agent command")
	}
	form := url.Values{}
	for k, v := range env.Arguments {
		form.Set(k, fmt.Sprint(v))
	}
	raw, err := p.c.postRaw(ctx, p.basePath(vm)+"/agent/"+env.Execute, form)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// --- helpers --------------------------------------------------------------

func (p *Provider) config(ctx context.Context, vm provider.VM) (map[string]any, error) {
	var cfg map[string]any
	if err := p.c.get(ctx, p.basePath(vm)+"/config", &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (p *Provider) agentHostname(ctx context.Context, vm provider.VM) (string, error) {
	var resp struct {
		Result struct {
			HostName string `json:"host-name"`
		} `json:"result"`
	}
	if err := p.c.get(ctx, p.basePath(vm)+"/agent/get-host-name", &resp); err != nil {
		return "", err
	}
	return resp.Result.HostName, nil
}

func (p *Provider) agentInterfaces(ctx context.Context, vm provider.VM) ([]agentIface, error) {
	var resp struct {
		Result []agentIface `json:"result"`
	}
	if err := p.c.get(ctx, p.basePath(vm)+"/agent/network-get-interfaces", &resp); err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (p *Provider) collectIPv4s(ctx context.Context, vm provider.VM, filter *provider.CIDRFilter) []string {
	ifs, err := p.agentInterfaces(ctx, vm)
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, iface := range ifs {
		if iface.Name == "lo" || iface.Name == "lo0" {
			continue
		}
		for _, a := range iface.IPAddresses {
			if a.Type != "ipv4" {
				continue
			}
			if !filter.Allow(a.Addr) {
				continue
			}
			seen[a.Addr] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for ip := range seen {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func parseConfigMACs(cfg map[string]any) []string {
	macs := []string{}
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		if strings.HasPrefix(k, "net") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		s, ok := cfg[k].(string)
		if !ok {
			continue
		}
		nd := parseNetEntry(k, s)
		if nd.MAC != "" {
			macs = append(macs, nd.MAC)
		}
	}
	return macs
}

// parseNetEntry pulls model + MAC + bridge out of strings like
// "virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0,firewall=1".
func parseNetEntry(key, s string) provider.NetDevice {
	nd := provider.NetDevice{Target: key, Type: "network"}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "virtio", "e1000", "rtl8139", "vmxnet3", "ne2k_pci", "ne2k_isa", "i82551", "i82557b", "i82559er", "pcnet":
			nd.Model = k
			nd.MAC = strings.ToUpper(v)
		case "bridge":
			nd.Source = "bridge=" + v
		}
	}
	return nd
}

// parseDiskEntry pulls source and size out of strings like
// "local-lvm:vm-100-disk-0,size=32G,iothread=1".
func parseDiskEntry(key, s string) provider.Disk {
	d := provider.Disk{Target: key, Bus: diskBusFromKey(key), Device: "disk"}
	parts := strings.Split(s, ",")
	if len(parts) > 0 {
		d.Source = parts[0]
		parts = parts[1:]
	}
	for _, part := range parts {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "size":
			if n, ok := parseHumanBytes(v); ok {
				d.Capacity = n
				d.Allocation = n
			}
		case "media":
			if v == "cdrom" {
				d.Device = "cdrom"
			}
		}
	}
	return d
}

func diskBusFromKey(k string) string {
	switch {
	case strings.HasPrefix(k, "scsi"):
		return "scsi"
	case strings.HasPrefix(k, "virtio"):
		return "virtio"
	case strings.HasPrefix(k, "ide"):
		return "ide"
	case strings.HasPrefix(k, "sata"):
		return "sata"
	case strings.HasPrefix(k, "efidisk"):
		return "efi"
	case strings.HasPrefix(k, "tpmstate"):
		return "tpm"
	}
	return ""
}

func isDiskKey(k string) bool {
	for _, prefix := range []string{"scsi", "virtio", "ide", "sata", "efidisk", "tpmstate"} {
		if strings.HasPrefix(k, prefix) {
			rest := k[len(prefix):]
			if rest == "" {
				return false
			}
			if _, err := strconv.Atoi(rest); err == nil {
				return true
			}
		}
	}
	return false
}

func parseHumanBytes(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	mult := uint64(1)
	switch s[len(s)-1] {
	case 'K', 'k':
		mult = 1024
		s = s[:len(s)-1]
	case 'M', 'm':
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case 'G', 'g':
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case 'T', 't':
		mult = 1024 * 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return uint64(n * float64(mult)), true
}
