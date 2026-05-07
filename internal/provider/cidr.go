package provider

import (
	"fmt"
	"net/netip"
	"strings"
)

// CIDRFilter excludes loopback and IPs that fall inside any user-supplied CIDR.
// A nil filter still drops loopback.
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

// Allow reports whether addr should be displayed.
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

// FormatIPv4 collapses a list of addresses to "first (+N)" unless wide is set.
func FormatIPv4(ips []string, wide bool) string {
	if len(ips) == 0 {
		return "-"
	}
	if wide || len(ips) == 1 {
		return strings.Join(ips, ",")
	}
	return fmt.Sprintf("%s (+%d)", ips[0], len(ips)-1)
}
