package kuberneteshelm

import (
	"io"
	"log/slog"
	"sync"

	"helm.sh/helm/v4/pkg/action"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type clientGetter struct {
	config    *rest.Config
	namespace string
	cache     discovery.CachedDiscoveryInterface
	once      sync.Once
	err       error
}

func (g *clientGetter) ToRESTConfig() (*rest.Config, error) { return rest.CopyConfig(g.config), nil }
func (g *clientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	g.once.Do(func() {
		client, e := discovery.NewDiscoveryClientForConfig(g.config)
		if e != nil {
			g.err = e
			return
		}
		g.cache = memory.NewMemCacheClient(client)
	})
	return g.cache, g.err
}
func (g *clientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	d, e := g.ToDiscoveryClient()
	if e != nil {
		return nil, e
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(d), nil
}
func (g *clientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	// No user-supplied paths, exec helpers or ambient kubeconfig merging.
	c := clientcmdapi.Config{CurrentContext: "argus", Contexts: map[string]*clientcmdapi.Context{"argus": {Cluster: "argus", AuthInfo: "argus", Namespace: g.namespace}}, Clusters: map[string]*clientcmdapi.Cluster{"argus": {Server: g.config.Host}}, AuthInfos: map[string]*clientcmdapi.AuthInfo{"argus": {}}}
	return clientcmd.NewNonInteractiveClientConfig(c, "argus", nil, nil)
}
func newConfiguration(config *rest.Config, namespace string) (*action.Configuration, error) {
	getter := &clientGetter{config: config, namespace: namespace}
	cfg := action.NewConfiguration()
	cfg.SetLogger(slog.NewTextHandler(io.Discard, nil))
	if e := cfg.Init(getter, namespace, "secret"); e != nil {
		return nil, ErrUnavailable
	}
	cfg.SetHookOutputFunc(func(string, string, string) io.Writer { return io.Discard })
	return cfg, nil
}
