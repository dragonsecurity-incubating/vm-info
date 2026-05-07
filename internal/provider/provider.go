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
	"time"
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

// Snapshot is one VM snapshot.
type Snapshot struct {
	Name        string
	Description string
	State       string // libvirt: domain state at snapshot; proxmox: "running"/"stopped"
	CreatedAt   time.Time
	Parent      string
	HasMemory   bool
	Current     bool
}

// SnapshotOpts shapes a `snapshot create` call.
type SnapshotOpts struct {
	Description string
	// Memory asks for memory state to be included. On libvirt this is the
	// default for running guests; on Proxmox it maps to vmstate=1.
	Memory bool
}

// Backup is one VM backup. Different backends populate different fields.
type Backup struct {
	ID        string // proxmox volid, e.g. "local:backup/vzdump-qemu-100-...vma.zst"
	Name      string // human-friendly file name
	Format    string // "vma.zst", "tar.gz", ...
	Size      uint64 // bytes
	CreatedAt time.Time
	Storage   string // proxmox storage name
	Notes     string
}

// BackupOpts shapes a `backup create` call.
type BackupOpts struct {
	Storage  string // proxmox: required (or "first backup-capable storage")
	Mode     string // proxmox: "snapshot" / "suspend" / "stop"; default snapshot
	Compress string // proxmox: "zstd" / "gzip" / "lzo" / "" (none); default zstd
	Notes    string
}

// Stats is a point-in-time sample of resource counters for one VM. CPU time
// is cumulative nanoseconds (Linux jiffy-style); disk and network counters
// are cumulative bytes. CPUPercent is filled when the backend already
// computes the rolling-average (Proxmox does); HasCPUPercent flags that.
type Stats struct {
	SampledAt     time.Time
	CPUTimeNanos  uint64
	CPUPercent    float64
	HasCPUPercent bool
	VCPUs         int

	MemTotalBytes uint64
	MemUsedBytes  uint64

	DiskReadBytes  uint64
	DiskWriteBytes uint64

	NetRXBytes uint64
	NetTXBytes uint64
}

// ResizeFlags controls whether resize ops apply to the live VM, the
// persistent config, or both. With both false, the backend's default
// applies (live for a running guest, config otherwise).
type ResizeFlags struct {
	Live   bool
	Config bool
}

// MigrateOpts shapes a `migrate` call.
type MigrateOpts struct {
	// Live attempts a live (no-pause) migration when the backend supports it.
	Live bool
	// Online is a Proxmox-specific synonym for "running guest, no shutdown";
	// if Online is true the request maps to online=1 in /migrate.
	Online bool
}

// TaskStatus is a generic async task report (Proxmox UPID-style). Libvirt
// operations are synchronous, so its provider returns ErrNotSupported.
type TaskStatus struct {
	UPID       string
	Type       string
	Status     string // "running" / "stopped"
	Running    bool
	ExitStatus string // "OK" on success
	StartTime  time.Time
	Node       string
	ID         string
	User       string
}

// TaskLogLine is one numbered log line from a task.
type TaskLogLine struct {
	N    int
	Text string
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

	ListSnapshots(ctx context.Context, vm VM) ([]Snapshot, error)
	CreateSnapshot(ctx context.Context, vm VM, name string, opts SnapshotOpts) error
	DeleteSnapshot(ctx context.Context, vm VM, name string) error
	RevertSnapshot(ctx context.Context, vm VM, name string) error

	ListBackups(ctx context.Context, vm VM) ([]Backup, error)
	CreateBackup(ctx context.Context, vm VM, opts BackupOpts) (string, error)
	DeleteBackup(ctx context.Context, vm VM, id string) error

	TaskStatus(ctx context.Context, upid string) (TaskStatus, error)
	TaskLog(ctx context.Context, upid string, start int) ([]TaskLogLine, error)

	Stats(ctx context.Context, vm VM) (Stats, error)
	GetAutostart(ctx context.Context, vm VM) (bool, error)
	SetAutostart(ctx context.Context, vm VM, on bool) error
	Reset(ctx context.Context, vm VM) error
	SetVCPUs(ctx context.Context, vm VM, count int, flags ResizeFlags) error
	SetMemoryMiB(ctx context.Context, vm VM, mib uint64, flags ResizeFlags) error
	ResizeDisk(ctx context.Context, vm VM, target string, sizeBytes uint64) error
	Migrate(ctx context.Context, vm VM, dest string, opts MigrateOpts) error
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
