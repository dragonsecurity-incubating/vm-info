package libvirt

import (
	"context"
	"fmt"

	"github.com/dragonsecurity/vm-info/internal/provider"
)

// libvirt operations are synchronous over the RPC, so there's nothing to
// poll for; surface that with a clear ErrNotSupported.

func (p *Provider) TaskStatus(_ context.Context, _ string) (provider.TaskStatus, error) {
	return provider.TaskStatus{}, fmt.Errorf("%w: libvirt operations are synchronous; nothing to watch",
		provider.ErrNotSupported)
}

func (p *Provider) TaskLog(_ context.Context, _ string, _ int) ([]provider.TaskLogLine, error) {
	return nil, fmt.Errorf("%w: libvirt has no task log", provider.ErrNotSupported)
}
