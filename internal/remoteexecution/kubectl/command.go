// Package kubectl builds the upstream CLI with a fixed, credential-free local
// connection. Connection selection flags cannot replace the approved resource.
package kubectl

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/rest"
	upstream "k8s.io/kubectl/pkg/cmd"
)

var connectionFlags = []string{"server", "kubeconfig", "context", "cluster", "user", "username", "password", "token", "as", "as-uid", "as-group", "certificate-authority", "client-certificate", "client-key", "insecure-skip-tls-verify", "tls-server-name"}

func Run(args []string, streams genericiooptions.IOStreams) error {
	proxy, err := proxyURL(os.Getenv("ARGUS_KUBECTL_PROXY"))
	if err != nil {
		return err
	}
	flags := genericclioptions.NewConfigFlags(false)
	var command *cobra.Command
	changed := func() bool {
		if command == nil {
			return false
		}
		for _, name := range connectionFlags {
			if f := command.PersistentFlags().Lookup(name); f != nil && f.Changed {
				return true
			}
		}
		return false
	}
	flags.WithWrapConfigFn(func(original *rest.Config) *rest.Config { return pinnedConfig(original, proxy, changed()) })
	// Plugins and kuberc aliases do not acquire another connection capability.
	command = upstream.NewKubectlCommand(upstream.KubectlOptions{ConfigFlags: flags, IOStreams: streams, Arguments: append([]string{"kubectl"}, args...)})
	before := command.PersistentPreRunE
	command.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if changed() {
			return errors.New("REMOTE_CONNECTION_OVERRIDE_FORBIDDEN")
		}
		if before != nil {
			return before(cmd, args)
		}
		return nil
	}
	command.SetArgs(args)
	command.SetIn(streams.In)
	command.SetOut(streams.Out)
	command.SetErr(streams.ErrOut)
	return command.Execute()
}
func proxyURL(value string) (*url.URL, error) {
	if value == "" {
		return nil, nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("REMOTE_BINDING_INVALID")
	}
	return u, nil
}
func pinnedConfig(original *rest.Config, proxy *url.URL, rejected bool) *rest.Config {
	address := "http://127.0.0.1:1"
	if proxy != nil {
		address = proxy.String()
	}
	// Copy only content settings; no tokens, exec plugins, certificates, proxy,
	// impersonation or transport inherited from user-editable kubeconfig.
	config := &rest.Config{Host: address, ContentConfig: original.ContentConfig, UserAgent: original.UserAgent, QPS: original.QPS, Burst: original.Burst, Timeout: 30 * time.Second}
	config.WrapTransport = func(base http.RoundTripper) http.RoundTripper {
		return pinnedTransport{base: base, proxy: proxy, rejected: rejected}
	}
	config.Dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		if proxy == nil || rejected {
			return nil, errors.New("REMOTE_CONNECTION_OVERRIDE_FORBIDDEN")
		}
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, proxy.Host)
	}
	config.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	return config
}

type pinnedTransport struct {
	base     http.RoundTripper
	proxy    *url.URL
	rejected bool
}

func (p pinnedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if p.rejected || p.proxy == nil || request.URL.Scheme != p.proxy.Scheme || request.URL.Host != p.proxy.Host {
		return nil, errors.New("REMOTE_CONNECTION_OVERRIDE_FORBIDDEN")
	}
	for name := range request.Header {
		if strings.EqualFold(name, "Authorization") || strings.HasPrefix(strings.ToLower(name), "impersonate-") {
			return nil, errors.New("REMOTE_CONNECTION_OVERRIDE_FORBIDDEN")
		}
	}
	return p.base.RoundTrip(request)
}
