// Package artifacthttp defines the HTTPS transport shared by artifact probes
// and downloads. Internal routing changes only the TCP destination, never TLS
// identity, HTTP authority, object path, or the immutable release metadata.
package artifacthttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/tlsmaterial"
)

type Route struct {
	Origin      string
	DialAddress string
}

func FromEnvironment(caPath string) (*http.Client, error) {
	route := Route{Origin: os.Getenv("ARGUS_ARTIFACT_INTERNAL_ORIGIN"), DialAddress: os.Getenv("ARGUS_ARTIFACT_INTERNAL_ADDRESS")}
	if (route.Origin == "") != (route.DialAddress == "") {
		return nil, errors.New("artifact internal origin and address must be configured together")
	}
	if route.Origin == "" {
		route.DialAddress = os.Getenv("ARGUS_ARTIFACT_DIAL_ADDRESS")
	}
	return New(caPath, route)
}

func New(caPath string, route Route) (*http.Client, error) {
	var origin *url.URL
	if route.Origin != "" {
		var err error
		origin, err = url.Parse(route.Origin)
		if err != nil || !validURL(origin) || origin.Path != "" || origin.RawQuery != "" {
			return nil, errors.New("artifact origin must be an HTTPS origin")
		}
	}
	if route.DialAddress != "" {
		host, port, err := net.SplitHostPort(route.DialAddress)
		if err != nil || host == "" || port == "" {
			return nil, errors.New("artifact dial address must be host:port")
		}
	}
	material, err := tlsmaterial.Load(tlsmaterial.Options{CABundlePath: caPath})
	if err != nil {
		return nil, err
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	if route.DialAddress != "" {
		base.Proxy = nil
		dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		base.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, route.DialAddress)
		}
	}
	transport, err := tlsmaterial.NewHTTPTransport(material, base)
	if err != nil {
		return nil, err
	}
	return &http.Client{Timeout: 2 * time.Minute, Transport: &originTransport{origin: origin, transport: transport},
		CheckRedirect: func(next *http.Request, previous []*http.Request) error {
			if len(previous) >= 3 || !validURL(next.URL) || !sameOrigin(next.URL, previous[0].URL) {
				return errors.New("artifact redirect changed origin")
			}
			return nil
		}}, nil
}

type originTransport struct {
	origin    *url.URL
	transport http.RoundTripper
}

func (t *originTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if !validURL(request.URL) || t.origin != nil && !sameOrigin(request.URL, t.origin) {
		return nil, errors.New("artifact request origin is not allowed")
	}
	return t.transport.RoundTrip(request)
}
func validURL(value *url.URL) bool {
	return value != nil && value.Scheme == "https" && value.Hostname() != "" && value.User == nil && value.Fragment == ""
}
func sameOrigin(a, b *url.URL) bool { return a.Scheme == b.Scheme && strings.EqualFold(a.Host, b.Host) }
