package proxmox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dragonsecurity/vm-info/internal/provider"
)

// pveTaskStatus mirrors GET /nodes/{node}/tasks/{upid}/status.
type pveTaskStatus struct {
	UPID       string `json:"upid"`
	Type       string `json:"type"`
	Status     string `json:"status"`     // "running" / "stopped"
	ExitStatus string `json:"exitstatus"` // present when stopped; "OK" on success
	StartTime  int64  `json:"starttime"`
	Node       string `json:"node"`
	ID         string `json:"id"`
	User       string `json:"user"`
	PID        int    `json:"pid"`
}

// pveTaskLogLine mirrors one element of GET /nodes/{node}/tasks/{upid}/log.
type pveTaskLogLine struct {
	N int    `json:"n"`
	T string `json:"t"`
}

// nodeFromUPID parses the node name out of a Proxmox UPID. Format:
//
//	UPID:nodename:pid:pstart:starttime:type:id:user@realm:
func nodeFromUPID(upid string) (string, error) {
	parts := strings.Split(upid, ":")
	if len(parts) < 2 || parts[0] != "UPID" || parts[1] == "" {
		return "", fmt.Errorf("invalid UPID %q", upid)
	}
	return parts[1], nil
}

func (p *Provider) TaskStatus(ctx context.Context, upid string) (provider.TaskStatus, error) {
	node, err := nodeFromUPID(upid)
	if err != nil {
		return provider.TaskStatus{}, err
	}
	var s pveTaskStatus
	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", node, upid)
	if err := p.c.get(ctx, path, &s); err != nil {
		return provider.TaskStatus{}, err
	}
	return provider.TaskStatus{
		UPID:       s.UPID,
		Type:       s.Type,
		Status:     s.Status,
		Running:    s.Status == "running",
		ExitStatus: s.ExitStatus,
		StartTime:  time.Unix(s.StartTime, 0),
		Node:       s.Node,
		ID:         s.ID,
		User:       s.User,
	}, nil
}

func (p *Provider) TaskLog(ctx context.Context, upid string, start int) ([]provider.TaskLogLine, error) {
	node, err := nodeFromUPID(upid)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/nodes/%s/tasks/%s/log?start=%s&limit=500",
		node, upid, strconv.Itoa(start))
	var lines []pveTaskLogLine
	if err := p.c.get(ctx, path, &lines); err != nil {
		return nil, err
	}
	out := make([]provider.TaskLogLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, provider.TaskLogLine{N: l.N, Text: l.T})
	}
	return out, nil
}
