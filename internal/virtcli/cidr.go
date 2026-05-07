package virtcli

import (
	"fmt"
	"net/netip"
	"strings"
)

// CIDRFilter excludes loopback addresses and any IPs that fall inside one of
// the user-supplied CIDR ranges. Empty filter still drops loopback.
type CIDRFilter struct {
	prefixes []netip.Prefix
}

func NewCIDRFilter(cidrs []string) (*CIDRFilter, error) {
	f := &CIDRFilter{}
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		p, err := netip.ParsePrefix(c)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", c, err)
		}
		f.prefixes = append(f.prefixes, p)
	}
	return f, nil
}

// Allow returns true if the address should be displayed.
func (f *CIDRFilter) Allow(addr string) bool {
	a, err := netip.ParseAddr(addr)
	if err != nil {
		return false
	}
	if a.IsLoopback() {
		return false
	}
	if f == nil {
		return true
	}
	for _, p := range f.prefixes {
		if p.Contains(a) {
			return false
		}
	}
	return true
}
