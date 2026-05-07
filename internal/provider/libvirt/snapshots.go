package libvirt

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/dragonsecurity/vm-info/internal/provider"
)

// snapshotXML is the projection of libvirt's <domainsnapshot> XML.
type snapshotXML struct {
	XMLName      xml.Name           `xml:"domainsnapshot"`
	Name         string             `xml:"name"`
	Description  string             `xml:"description"`
	State        string             `xml:"state"`
	CreationTime int64              `xml:"creationTime"`
	Parent       snapshotXMLParent  `xml:"parent"`
	Memory       snapshotXMLMemory  `xml:"memory"`
}

type snapshotXMLParent struct {
	Name string `xml:"name"`
}

type snapshotXMLMemory struct {
	Snapshot string `xml:"snapshot,attr"`
	File     string `xml:"file,attr"`
}

func (p *Provider) ListSnapshots(_ context.Context, vm provider.VM) ([]provider.Snapshot, error) {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return nil, err
	}
	snaps, _, err := p.l.DomainListAllSnapshots(d, 1024, 0)
	if err != nil {
		return nil, err
	}

	var current string
	if has, _ := p.l.DomainHasCurrentSnapshot(d, 0); has != 0 {
		if c, err := p.l.DomainSnapshotCurrent(d, 0); err == nil {
			current = c.Name
		}
	}

	out := make([]provider.Snapshot, 0, len(snaps))
	for _, s := range snaps {
		desc, err := p.l.DomainSnapshotGetXMLDesc(s, 0)
		if err != nil {
			out = append(out, provider.Snapshot{Name: s.Name})
			continue
		}
		var sx snapshotXML
		_ = xml.Unmarshal([]byte(desc), &sx)
		out = append(out, provider.Snapshot{
			Name:        s.Name,
			Description: sx.Description,
			State:       sx.State,
			CreatedAt:   time.Unix(sx.CreationTime, 0),
			Parent:      sx.Parent.Name,
			HasMemory:   sx.Memory.Snapshot == "internal" || sx.Memory.File != "",
			Current:     s.Name == current,
		})
	}
	return out, nil
}

func (p *Provider) CreateSnapshot(_ context.Context, vm provider.VM, name string, opts provider.SnapshotOpts) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	desc := buildSnapshotXML(name, opts)
	_, err = p.l.DomainSnapshotCreateXML(d, desc, 0)
	return err
}

func (p *Provider) DeleteSnapshot(_ context.Context, vm provider.VM, name string) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	s, err := p.l.DomainSnapshotLookupByName(d, name, 0)
	if err != nil {
		return err
	}
	return p.l.DomainSnapshotDelete(s, 0)
}

func (p *Provider) RevertSnapshot(_ context.Context, vm provider.VM, name string) error {
	d, err := p.lookupDomain(vm.Name)
	if err != nil {
		return err
	}
	s, err := p.l.DomainSnapshotLookupByName(d, name, 0)
	if err != nil {
		return err
	}
	return p.l.DomainRevertToSnapshot(s, 0)
}

func buildSnapshotXML(name string, opts provider.SnapshotOpts) string {
	var sb strings.Builder
	sb.WriteString("<domainsnapshot>")
	fmt.Fprintf(&sb, "<name>%s</name>", xmlEscape(name))
	if opts.Description != "" {
		fmt.Fprintf(&sb, "<description>%s</description>", xmlEscape(opts.Description))
	}
	sb.WriteString("</domainsnapshot>")
	return sb.String()
}

func xmlEscape(s string) string {
	var sb strings.Builder
	_ = xml.EscapeText(&sb, []byte(s))
	return sb.String()
}

// --- backup --------------------------------------------------------------
//
// libvirt's native backup API (DomainBackupBegin) is incremental-only and
// requires NBD endpoint setup, push/pull configuration, and checkpoints —
// substantially more involved than a single API call. For now we surface
// ErrNotSupported with a pointer to external tooling.

func (p *Provider) ListBackups(_ context.Context, _ provider.VM) ([]provider.Backup, error) {
	return nil, fmt.Errorf("%w: libvirt has no backup catalog — use external tooling (virt-backup, virsh backup-begin) to manage backups",
		provider.ErrNotSupported)
}

func (p *Provider) CreateBackup(_ context.Context, _ provider.VM, _ provider.BackupOpts) (string, error) {
	return "", fmt.Errorf("%w: native libvirt backups require NBD setup; use 'snapshot create' for point-in-time copies, or virt-backup / virsh backup-begin for full backups",
		provider.ErrNotSupported)
}

func (p *Provider) DeleteBackup(_ context.Context, _ provider.VM, _ string) error {
	return fmt.Errorf("%w: libvirt has no backup catalog", provider.ErrNotSupported)
}

