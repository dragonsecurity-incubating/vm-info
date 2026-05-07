// Package provider abstracts a hypervisor backend behind a single interface
// so vm-info can target both libvirt and Proxmox VE. Backends register
// themselves against URI schemes (qemu*, pve, proxmox); Connect picks one.
package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

// ErrNotSupported is returned by a provider for operations that don't apply
// to its backend (for example, qemu-agent-command on a backend that doesn't
// proxy QGA).
var ErrNotSupported = errors.New("operation not supported by this provider")

// VM is the cross-backend identifier for a guest.
type VM struct {
	Name     string // human name (libvirt domain name / proxmox VM name)
	ID       string // libvirt domain ID, proxmox VMID — display string, may be "-"
	UUID     string // canonical UUID where available, empty otherwise
	Node     string // proxmox cluster node, "" for libvirt
	Provider string // "libvirt" or "proxmox"
}

// Info is everything the default vm-info table column-renders for one VM.
type Info struct {
	VM
	State    string
	Running  bool
	VCPUs    int
	RAMMiB   uint64
	MaxMiB   uint64
	Hostname string
	IPv4s    []string
	MACs     []string
}

// Interface is one row of `domifaddr`.
type Interface struct {
	Name     string
	MAC      string
	Protocol string // "ipv4" or "ipv6"
	Addr     string
	Prefix   uint32
}

// NetDevice is one row of `domiflist`.
type NetDevice struct {
	Target string
	Type   string
	Source string
	Model  string
	MAC    string
}

// Disk is one row of `domblklist`. Capacity/Allocation may be zero when the
// backend can't cheaply report them.
type Disk struct {
	Target     string
	Bus        string
	Source     string
	Type       string // "disk", "cdrom", ...
	Device     string
	Capacity   uint64
	Allocation uint64
}

// BlockInfo is the result of `domblkinfo`.
type BlockInfo struct {
	Capacity   uint64
	Allocation uint64
	Physical   uint64
}

// RebootOpts mirrors virsh's reboot flags.
type RebootOpts struct {
	ACPI bool
}

// Provider is the cross-backend hypervisor interface.
type Provider interface {
	Close() error
	Kind() string // "libvirt" / "proxmox"
	URI() string

	List(ctx context.Context) ([]VM, error)
	Lookup(ctx context.Context, name string) (VM, error)

	Info(ctx context.Context, vm VM, filter *CIDRFilter) (Info, error)
	State(ctx context.Context, vm VM) (string, error)
	Hostname(ctx context.Context, vm VM) (string, error)
	Interfaces(ctx context.Context, vm VM, source string) ([]Interface, error)
	NetDevices(ctx context.Context, vm VM) ([]NetDevice, error)
	Disks(ctx context.Context, vm VM) ([]Disk, error)
	BlockInfo(ctx context.Context, vm VM, target string) (BlockInfo, error)
	Config(ctx context.Context, vm VM, inactive bool) (string, error)

	Start(ctx context.Context, vm VM) error
	Shutdown(ctx context.Context, vm VM) error
	Destroy(ctx context.Context, vm VM) error
	Reboot(ctx context.Context, vm VM, opts RebootOpts) error
	Suspend(ctx context.Context, vm VM) error
	Resume(ctx context.Context, vm VM) error

	AgentCommand(ctx context.Context, vm VM, cmd string, timeoutSec int32) (string, error)
}

// Factory dials a provider for the supplied URI.
type Factory func(uri string) (Provider, error)

var factories = map[string]Factory{}

// Register associates a URI scheme with a factory. Called from provider
// package init() functions.
func Register(scheme string, f Factory) {
	factories[scheme] = f
}

// RegisteredSchemes returns the schemes currently supported, sorted.
func RegisteredSchemes() []string {
	out := make([]string, 0, len(factories))
	for s := range factories {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// DefaultURI returns the URI to use when --connect is not given. It honours
// VM_INFO_URI, then LIBVIRT_DEFAULT_URI, then qemu:///system.
func DefaultURI() string {
	if v := os.Getenv("VM_INFO_URI"); v != "" {
		return v
	}
	if v := os.Getenv("LIBVIRT_DEFAULT_URI"); v != "" {
		return v
	}
	return "qemu:///system"
}

// Connect dispatches to the registered factory for the URI's scheme.
func Connect(uri string) (Provider, error) {
	if uri == "" {
		uri = DefaultURI()
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse uri %q: %w", uri, err)
	}
	f, ok := factories[u.Scheme]
	if !ok {
		return nil, fmt.Errorf("unsupported URI scheme %q (registered: %s)",
			u.Scheme, strings.Join(RegisteredSchemes(), ", "))
	}
	return f(uri)
}
