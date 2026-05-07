// Package proxmox implements provider.Provider against the Proxmox VE REST API.
package proxmox

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// client is the low-level HTTP wrapper around Proxmox's /api2/json endpoint.
type client struct {
	base   *url.URL
	hc     *http.Client
	auth   authMode
	uri    string
	rawURI string
}

type authMode struct {
	// API token: full string "USER@REALM!TOKENID=SECRET"
	tokenHeader string
	// Ticket auth (fallback): cookie + CSRF
	ticket    string
	csrfToken string
}

// newClient builds an HTTP client for the supplied URI. Token auth comes from
// (in order of precedence): a ?token=... query param, or PVE_API_TOKEN env.
// ?insecure=1 disables TLS verification (Proxmox often uses self-signed certs).
func newClient(uri string) (*client, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse uri: %w", err)
	}
	host := u.Host
	if host == "" {
		return nil, fmt.Errorf("proxmox URI must include host: got %q", uri)
	}
	if !strings.Contains(host, ":") {
		host += ":8006"
	}

	q := u.Query()
	insecure := q.Get("insecure") == "1" || os.Getenv("PVE_INSECURE") == "1"

	base := &url.URL{Scheme: "https", Host: host, Path: "/api2/json"}

	hc := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec
		},
	}

	c := &client{base: base, hc: hc, uri: u.Redacted(), rawURI: uri}

	token := q.Get("token")
	if token == "" {
		token = os.Getenv("PVE_API_TOKEN")
	}
	if token != "" {
		c.auth.tokenHeader = "PVEAPIToken=" + token
		return c, nil
	}

	return nil, fmt.Errorf("proxmox: no API token supplied — pass ?token=USER@REALM!TOKENID=SECRET in the URI or set PVE_API_TOKEN")
}

func (c *client) do(ctx context.Context, method, path string, form url.Values, out any) error {
	endpoint := *c.base
	endpoint.Path = c.base.Path + path

	var body io.Reader
	if method != http.MethodGet && form != nil {
		body = strings.NewReader(form.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return err
	}
	if c.auth.tokenHeader != "" {
		req.Header.Set("Authorization", c.auth.tokenHeader)
	}
	if c.auth.ticket != "" {
		req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: c.auth.ticket})
		if method != http.MethodGet && c.auth.csrfToken != "" {
			req.Header.Set("CSRFPreventionToken", c.auth.csrfToken)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("proxmox %s %s: %d %s", method, path, resp.StatusCode, summarize(raw))
	}
	if out == nil {
		return nil
	}
	wrapper := struct {
		Data json.RawMessage `json:"data"`
	}{}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return fmt.Errorf("decode response: %w (body: %s)", err, summarize(raw))
	}
	if len(wrapper.Data) == 0 || string(wrapper.Data) == "null" {
		return nil
	}
	return json.Unmarshal(wrapper.Data, out)
}

func (c *client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *client) post(ctx context.Context, path string, form url.Values) error {
	return c.do(ctx, http.MethodPost, path, form, nil)
}

func summarize(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 256 {
		s = s[:256] + "…"
	}
	return s
}

// peek lets us reuse do() for raw JSON inspection (used by AgentCommand).
func (c *client) postRaw(ctx context.Context, path string, form url.Values) ([]byte, error) {
	endpoint := *c.base
	endpoint.Path = c.base.Path + path
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	if c.auth.tokenHeader != "" {
		req.Header.Set("Authorization", c.auth.tokenHeader)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("proxmox POST %s: %d %s", path, resp.StatusCode, summarize(raw))
	}
	return raw, nil
}

// drain is a tiny no-op to keep the bytes import in case body capture is added.
var _ = bytes.NewReader
