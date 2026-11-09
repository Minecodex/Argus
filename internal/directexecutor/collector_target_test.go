package directexecutor

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/resource"
)

type collectorTargetResolver struct {
	hostname string
}

func (resolver collectorTargetResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if host != resolver.hostname {
		return nil, errors.New("unexpected DNS hostname")
	}
	return []netip.Addr{netip.MustParseAddr("192.168.10.5")}, nil
}

func TestCollectorTargetAddressesUsesProtocolEndpoint(t *testing.T) {
	tests := []struct {
		name, resourceType, address string
		denied                      bool
	}{
		{"Kubernetes HTTPS with port", "kubernetes_cluster", "https://cluster.example:22443", false},
		{"SSH hostname", "host", "cluster.example", false},
		{"Kubernetes plain HTTP", "kubernetes_cluster", "http://cluster.example:22443", true},
		{"Kubernetes missing scheme", "kubernetes_cluster", "cluster.example", true},
		{"Kubernetes embedded credentials", "kubernetes_cluster", "https://user:pass@cluster.example", true},
		{"Kubernetes loopback", "kubernetes_cluster", "https://127.0.0.1:22443", true},
		{"Kubernetes metadata", "kubernetes_cluster", "https://169.254.169.254", true},
		{"SSH loopback", "host", "127.0.0.1", true},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			executor := &Executor{Validator: resource.DirectTargetValidator{
				Resolver: collectorTargetResolver{hostname: "cluster.example"},
			}}
			addresses, err := executor.collectorTargetAddresses(t.Context(), &connectorv1.CollectorManagementCommand{
				ResourceType: item.resourceType, TargetAddress: item.address,
			})
			if item.denied {
				if !errors.Is(err, resource.ErrDirectTargetDenied) {
					t.Fatalf("expected target denial, got addresses %v, error %v", addresses, err)
				}
				return
			}
			if err != nil || len(addresses) != 1 || addresses[0] != netip.MustParseAddr("192.168.10.5") {
				t.Fatalf("expected validated DNS address, got %v, error %v", addresses, err)
			}
		})
	}
}
