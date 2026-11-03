package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// An explicit E2E-only public DNS override accommodates development Fake-IP
// resolvers. The provider's SSRF checks and TLS hostname verification stay on.
type realModelEndpointResolver struct{ address netip.Addr }

func (r realModelEndpointResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{r.address}, nil
}

func validateRealModelEndpointIP(config p5RealModelConfig) error {
	if config.EndpointIP == "" {
		return nil
	}
	address, err := netip.ParseAddr(config.EndpointIP)
	if err != nil {
		return fmt.Errorf("%w: endpoint_ip must be a public IP address", errUsage)
	}
	policy := modelprovider.PublicEndpointPolicy{Resolver: realModelEndpointResolver{address}}
	if _, err := policy.Validate(context.Background(), config.BaseURL); err != nil {
		return fmt.Errorf("%w: endpoint_ip must satisfy the model public endpoint policy", errUsage)
	}
	return nil
}

func configureRealModelEndpointDNS(ctx context.Context, env *E2EEnvironment) error {
	config := env.Options.RealModel
	if config == nil || config.EndpointIP == "" {
		return nil
	}
	if err := validateRealModelEndpointIP(*config); err != nil {
		return err
	}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || env.ReleaseID == "" || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("real-model DNS override requires the owned E2E system namespace")
	}
	endpoint, _ := url.Parse(config.BaseURL)
	deployments, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	patched := []string{}
	for _, deployment := range deployments.Items {
		if deployment.Name != "argus-server" && deployment.Name != "argus-worker" && !strings.HasPrefix(deployment.Name, "argus-worker-") {
			continue
		}
		aliases := []corev1.HostAlias{}
		for _, alias := range deployment.Spec.Template.Spec.HostAliases {
			hosts := []string{}
			for _, hostname := range alias.Hostnames {
				if hostname != endpoint.Hostname() {
					hosts = append(hosts, hostname)
				}
			}
			if len(hosts) > 0 {
				alias.Hostnames = hosts
				aliases = append(aliases, alias)
			}
		}
		deployment.Spec.Template.Spec.HostAliases = append(aliases, corev1.HostAlias{IP: config.EndpointIP, Hostnames: []string{endpoint.Hostname()}})
		if config.DisableMLKEM {
			for i := range deployment.Spec.Template.Spec.Containers {
				container := &deployment.Spec.Template.Spec.Containers[i]
				found := false
				for j := range container.Env {
					if container.Env[j].Name == "GODEBUG" {
						if container.Env[j].ValueFrom != nil {
							return fmt.Errorf("cannot override referenced GODEBUG in owned E2E deployment")
						}
						parts := []string{}
						for _, part := range strings.Split(container.Env[j].Value, ",") {
							if part != "" && !strings.HasPrefix(part, "tlsmlkem=") {
								parts = append(parts, part)
							}
						}
						container.Env[j].Value = strings.Join(append(parts, "tlsmlkem=0"), ",")
						found = true
					}
				}
				if !found {
					container.Env = append(container.Env, corev1.EnvVar{Name: "GODEBUG", Value: "tlsmlkem=0"})
				}
			}
		}
		if _, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Update(ctx, &deployment, metav1.UpdateOptions{}); err != nil {
			return err
		}
		patched = append(patched, deployment.Name)
	}
	if len(patched) < 2 {
		return fmt.Errorf("real-model DNS override did not find server and worker deployments")
	}
	for _, name := range patched {
		if err := env.Kube.WaitDeployment(ctx, env.SystemNS, name, 5*time.Minute); err != nil {
			return err
		}
	}
	data, _ := json.MarshalIndent(map[string]any{"hostname": endpoint.Hostname(), "public_ip": config.EndpointIP, "deployments": patched, "tls_mlkem_disabled": config.DisableMLKEM, "scope": "owned temporary E2E namespace only; TLS certificate verification and public endpoint policy preserved"}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "real-model-dns.json"), data)
}
