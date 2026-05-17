package proxmox

import (
	"context"
	"net/url"

	"github.com/dragonsecurity/vm-info/internal/provider"
)

// AttachISO mounts source into the cdrom slot. slot is the config key
// (ide2, scsi3, sata0, ...); source is a PVE volume id like
// "local:iso/debian-12.iso".
func (p *Provider) AttachISO(ctx context.Context, vm provider.VM, slot, source string) error {
	form := url.Values{}
	form.Set(slot, source+",media=cdrom")
	return p.c.do(ctx, "PUT", p.basePath(vm)+"/config", form, nil)
}

// EjectISO leaves the cdrom slot in place but unloads the media (PVE's
// canonical form is `<slot>=none,media=cdrom`).
func (p *Provider) EjectISO(ctx context.Context, vm provider.VM, slot string) error {
	form := url.Values{}
	form.Set(slot, "none,media=cdrom")
	return p.c.do(ctx, "PUT", p.basePath(vm)+"/config", form, nil)
}
