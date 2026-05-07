package virtcli

import (
	"fmt"
	"net/url"
	"os"

	"github.com/digitalocean/go-libvirt"
)

// DefaultURI returns the URI used when --connect is not supplied. It honours
// LIBVIRT_DEFAULT_URI, falling back to qemu:///system, matching virsh.
func DefaultURI() string {
	if v := os.Getenv("LIBVIRT_DEFAULT_URI"); v != "" {
		return v
	}
	return "qemu:///system"
}

// Connect dials libvirt at the given URI. The URI accepts the same forms as
// virsh -c (qemu:///system, qemu:///session, qemu+ssh://..., qemu+tcp://...,
// qemu+tls://..., qemu+libssh://...).
func Connect(uri string) (*libvirt.Libvirt, error) {
	if uri == "" {
		uri = DefaultURI()
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse uri %q: %w", uri, err)
	}
	l, err := libvirt.ConnectToURI(u)
	if err != nil {
		return nil, err
	}
	return l, nil
}
