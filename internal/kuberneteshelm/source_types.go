package kuberneteshelm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/netip"
	"net/url"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type ChartSource struct {
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	Chart     string `json:"chart,omitempty"`
	Version   string `json:"version"`
	Digest    string `json:"digest,omitempty"`
	PlainHTTP bool   `json:"plain_http,omitempty"`
	AuthURL   string `json:"auth_url,omitempty"`
}

type SourceCredentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
	CAPEM    string `json:"ca_pem,omitempty"`
}

type ChartProvenance struct {
	Transport     string `json:"transport,omitempty"`
	AuthURL       string `json:"auth_url,omitempty"`
	Kind          string `json:"kind"`
	Origin        string `json:"origin,omitempty"`
	Chart         string `json:"chart"`
	Version       string `json:"version"`
	Digest        string `json:"digest"`
	ArchiveDigest string `json:"archive_digest"`
}

type ChartNetworkPolicy interface {
	Resolve(context.Context, string) ([]netip.Addr, error)
	Revalidate(context.Context, string, []netip.Addr) error
}

func DecodeRequest(raw []byte, r *Request) error {
	if len(raw) > 768<<10 {
		return ErrBudget
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(r) != nil || d.Decode(new(any)) != io.EOF || ValidateChartSource(r.Source) != nil {
		return ErrInvalid
	}
	return nil
}

var chartNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
var sourceDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func ValidateChartSource(s *ChartSource) error {
	if s == nil {
		return nil
	}
	if s.Kind != "repository" && s.Kind != "oci" || len(s.Version) > 128 || len(s.URL) > 2048 || len(s.AuthURL) > 2048 {
		return ErrInvalid
	}
	if _, e := semver.StrictNewVersion(s.Version); e != nil || s.Digest != "" && !sourceDigestPattern.MatchString(s.Digest) {
		return ErrInvalid
	}
	raw := s.URL
	if s.Kind == "oci" {
		if !strings.HasPrefix(raw, "oci://") {
			return ErrInvalid
		}
		raw = "https://" + strings.TrimPrefix(raw, "oci://")
	}
	u, e := sourceURL(raw, s.PlainHTTP)
	if e != nil {
		return e
	}
	if s.Kind == "repository" && !chartNamePattern.MatchString(s.Chart) {
		return ErrInvalid
	}
	if s.Kind == "oci" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 0 || !chartNamePattern.MatchString(parts[len(parts)-1]) || strings.ContainsAny(u.Path, "@:") || s.Chart != "" && s.Chart != parts[len(parts)-1] {
			return ErrInvalid
		}
	}
	if s.AuthURL != "" {
		if s.Kind != "oci" {
			return ErrInvalid
		}
		if _, e := sourceURL(s.AuthURL, s.PlainHTTP); e != nil {
			return e
		}
	}
	return nil
}

func ValidateSourceCredentials(c *SourceCredentials) error {
	if c == nil {
		return nil
	}
	if len(c.Username) > 256 || len(c.Password) > 4096 || len(c.Token) > 16384 || len(c.CAPEM) > 32768 || strings.ContainsAny(c.Username+c.Password+c.Token, "\x00\r\n") || strings.Contains(c.Username, ":") || strings.Contains(c.CAPEM, "PRIVATE KEY") {
		return ErrInvalid
	}
	if (c.Username == "") != (c.Password == "") || c.Token != "" && (c.Username != "" || c.Password != "") {
		return ErrInvalid
	}
	return nil
}

func (c *SourceCredentials) Clear() {
	if c != nil {
		*c = SourceCredentials{}
	}
}

func (m *Material) Clear() {
	clear(m.Chart)
	clear(m.Values)
	clear(m.Key)
	if m.SourceCredentials != nil {
		m.SourceCredentials.Clear()
	}
}

func sourceURL(raw string, plain bool) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || u == nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(raw, "\x00\r\n\\ ") || strings.ContainsAny(u.Path, "\x00\r\n\\") || u.Scheme != "https" && !(plain && u.Scheme == "http") {
		return nil, ErrInvalid
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return nil, ErrInvalid
		}
	}
	return u, nil
}
