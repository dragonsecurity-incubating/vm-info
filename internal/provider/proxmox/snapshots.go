package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/dragonsecurity/vm-info/internal/provider"
)

// pveSnap is one entry in GET /nodes/{node}/qemu/{vmid}/snapshot.
type pveSnap struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SnapTime    int64  `json:"snaptime"`
	Parent      string `json:"parent"`
	VMState     int    `json:"vmstate"`
	Running     int    `json:"running"`
}

func (p *Provider) ListSnapshots(ctx context.Context, vm provider.VM) ([]provider.Snapshot, error) {
	var snaps []pveSnap
	if err := p.c.get(ctx, p.basePath(vm)+"/snapshot", &snaps); err != nil {
		return nil, err
	}
	out := make([]provider.Snapshot, 0, len(snaps))
	for _, s := range snaps {
		// Proxmox includes a synthetic "current" entry for the live state;
		// it has no snaptime. Mark it as Current rather than dropping it,
		// since that's how virsh exposes the same info.
		current := s.Name == "current"
		state := "stopped"
		if s.Running == 1 {
			state = "running"
		}
		out = append(out, provider.Snapshot{
			Name:        s.Name,
			Description: s.Description,
			State:       state,
			CreatedAt:   time.Unix(s.SnapTime, 0),
			Parent:      s.Parent,
			HasMemory:   s.VMState == 1,
			Current:     current,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return !out[i].Current
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (p *Provider) CreateSnapshot(ctx context.Context, vm provider.VM, name string, opts provider.SnapshotOpts) error {
	form := url.Values{}
	form.Set("snapname", name)
	if opts.Description != "" {
		form.Set("description", opts.Description)
	}
	if opts.Memory {
		form.Set("vmstate", "1")
	}
	return p.c.post(ctx, p.basePath(vm)+"/snapshot", form)
}

func (p *Provider) DeleteSnapshot(ctx context.Context, vm provider.VM, name string) error {
	return p.c.do(ctx, "DELETE", p.basePath(vm)+"/snapshot/"+name, nil, nil)
}

func (p *Provider) RevertSnapshot(ctx context.Context, vm provider.VM, name string) error {
	return p.c.post(ctx, p.basePath(vm)+"/snapshot/"+name+"/rollback", nil)
}

// --- backups -------------------------------------------------------------

// pveStorage is one entry in GET /nodes/{node}/storage.
type pveStorage struct {
	Storage string `json:"storage"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Active  int    `json:"active"`
}

// pveBackup is one entry in GET /nodes/{node}/storage/{storage}/content.
type pveBackup struct {
	VolID  string `json:"volid"`
	Format string `json:"format"`
	Size   uint64 `json:"size"`
	CTime  int64  `json:"ctime"`
	Notes  string `json:"notes"`
	VMID   int    `json:"vmid"`
}

func (p *Provider) listBackupStorages(ctx context.Context, node string) ([]pveStorage, error) {
	var st []pveStorage
	if err := p.c.get(ctx, "/nodes/"+node+"/storage", &st); err != nil {
		return nil, err
	}
	out := st[:0]
	for _, s := range st {
		if s.Active == 0 {
			continue
		}
		// content is comma-separated: "rootdir,images,backup"
		for _, c := range commaSplit(s.Content) {
			if c == "backup" {
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}

func (p *Provider) ListBackups(ctx context.Context, vm provider.VM) ([]provider.Backup, error) {
	stores, err := p.listBackupStorages(ctx, vm.Node)
	if err != nil {
		return nil, err
	}
	var out []provider.Backup
	for _, s := range stores {
		var bs []pveBackup
		path := fmt.Sprintf("/nodes/%s/storage/%s/content?content=backup&vmid=%s",
			vm.Node, s.Storage, vm.ID)
		if err := p.c.get(ctx, path, &bs); err != nil {
			continue
		}
		for _, b := range bs {
			out = append(out, provider.Backup{
				ID:        b.VolID,
				Name:      stripStoragePrefix(b.VolID),
				Format:    b.Format,
				Size:      b.Size,
				CreatedAt: time.Unix(b.CTime, 0),
				Storage:   s.Storage,
				Notes:     b.Notes,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (p *Provider) CreateBackup(ctx context.Context, vm provider.VM, opts provider.BackupOpts) (string, error) {
	storage := opts.Storage
	if storage == "" {
		stores, err := p.listBackupStorages(ctx, vm.Node)
		if err != nil {
			return "", err
		}
		if len(stores) == 0 {
			return "", fmt.Errorf("no backup-capable storage on node %s — pass --storage", vm.Node)
		}
		storage = stores[0].Storage
	}

	mode := opts.Mode
	if mode == "" {
		mode = "snapshot"
	}
	compress := opts.Compress
	if compress == "" {
		compress = "zstd"
	}

	form := url.Values{}
	form.Set("vmid", vm.ID)
	form.Set("storage", storage)
	form.Set("mode", mode)
	if compress != "none" {
		form.Set("compress", compress)
	}
	if opts.Notes != "" {
		form.Set("notes-template", opts.Notes)
	}

	// vzdump returns a UPID (task ID) in the data field.
	var upid string
	if err := p.c.do(ctx, "POST", "/nodes/"+vm.Node+"/vzdump", form, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

func (p *Provider) DeleteBackup(ctx context.Context, vm provider.VM, id string) error {
	storage, _, ok := splitVolID(id)
	if !ok {
		return fmt.Errorf("invalid backup id %q (expected storage:backup/...)", id)
	}
	path := fmt.Sprintf("/nodes/%s/storage/%s/content/%s",
		vm.Node, storage, urlEscapeVolID(id))
	return p.c.do(ctx, "DELETE", path, nil, nil)
}

// --- helpers --------------------------------------------------------------

func commaSplit(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if start < i {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	return out
}

func splitVolID(volid string) (storage, file string, ok bool) {
	for i := 0; i < len(volid); i++ {
		if volid[i] == ':' {
			return volid[:i], volid[i+1:], true
		}
	}
	return "", "", false
}

func stripStoragePrefix(volid string) string {
	if _, file, ok := splitVolID(volid); ok {
		return file
	}
	return volid
}

func urlEscapeVolID(volid string) string {
	return url.PathEscape(volid)
}
