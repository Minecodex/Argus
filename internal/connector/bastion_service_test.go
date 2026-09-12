package connector

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestValidateBastionRelayStatusUsesStructuredReportedPorts(t *testing.T) {
	status := &connectorv1.BastionRelayStatus{Status: "ready", Generation: 1, AdvertiseAddress: "10.0.0.8", Listeners: []*connectorv1.BastionRelayListener{
		{Kind: "https", BindAddress: "[::]:8447", Port: 8447, Status: "ready"},
		{Kind: "gateway", BindAddress: "[::]:9448", Port: 9448, Status: "ready"},
	}}
	address, httpsPort, gatewayPort, generation, err := validateBastionRelayStatus(status)
	if err != nil || address != "10.0.0.8" || httpsPort != 8447 || gatewayPort != 9448 || generation != 1 {
		t.Fatalf("unexpected validated relay status: address=%q https=%d gateway=%d generation=%d err=%v", address, httpsPort, gatewayPort, generation, err)
	}
	status.Listeners[1].Kind = "https"
	if _, _, _, _, err = validateBastionRelayStatus(status); err == nil {
		t.Fatal("duplicate relay listener kind was accepted")
	}
}

func TestNewBastionServiceRequiresSharedRuntimeInputs(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{0x42}, 32)
	store := new(postgres.Store)
	for name, candidate := range map[string]struct {
		store         *postgres.Store
		key           []byte
		enrollTarget  string
		gatewayTarget string
	}{
		"store":          {key: key, enrollTarget: "enroll:8443", gatewayTarget: "gateway:9443"},
		"key":            {store: store, key: key[:31], enrollTarget: "enroll:8443", gatewayTarget: "gateway:9443"},
		"enroll target":  {store: store, key: key, gatewayTarget: "gateway:9443"},
		"gateway target": {store: store, key: key, enrollTarget: "enroll:8443"},
	} {
		candidate := candidate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewBastionService(candidate.store, resource.PendingActionService{}, Service{}, candidate.key,
				candidate.enrollTarget, candidate.gatewayTarget); err == nil {
				t.Fatal("incomplete runtime configuration was accepted")
			}
		})
	}
	service, err := NewBastionService(store, resource.PendingActionService{}, Service{}, key, "enroll:8443", "gateway:9443")
	if err != nil {
		t.Fatal(err)
	}
	key[0] ^= 0xff
	if service.OperationSecretKey[0] == key[0] {
		t.Fatal("operation secret key aliases caller memory")
	}
}

func TestBastionSnapshotStatusReplacementAllowsOfflineTransition(t *testing.T) {
	for _, status := range []string{"active", "suspected_offline", "offline", "uninstalled"} {
		if got := bastionSnapshotStatus("replace", status); got != "replaceable" {
			t.Fatalf("replace status %q normalized to %q", status, got)
		}
	}
}

func TestReplacementStatusRejectsPendingScope(t *testing.T) {
	if replacementStatusAllowed("pending") {
		t.Fatal("pending scope without an active Connector must not be replaceable")
	}
}

func TestBastionSnapshotStatusOtherOperationsRemainExact(t *testing.T) {
	if got := bastionSnapshotStatus("delete", "offline"); got != "offline" {
		t.Fatalf("delete status normalized to %q", got)
	}
}

func TestBaselineLetsReplacementReuseStableInstanceID(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	migration, err := os.ReadFile(filepath.Join(root, "migrations", "postgresql", "00001_argus_baseline.sql"))
	if err != nil {
		t.Fatal(err)
	}
	up := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"CREATE UNIQUE INDEX connectors_enterprise_instance_live_unique ON public.connectors",
		"WHERE (status <> ALL (ARRAY['revoked'::text, 'uninstalled'::text]))",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("Connector replacement baseline is missing %q", required)
		}
	}
}

func TestBastionRootHostLifecycleKeepsDeletedNamesReusable(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	base, err := os.ReadFile(filepath.Join(root, "migrations", "postgresql", "00001_argus_baseline.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"bastion_scopes_name_unique ON public.bastion_scopes",
		"hosts_name_unique ON public.hosts",
		"WHERE (status <> 'deleted'::text)",
	} {
		if !strings.Contains(string(base), required) {
			t.Fatalf("live-name uniqueness is missing %q", required)
		}
	}

	for _, required := range []string{
		"CREATE UNIQUE INDEX hosts_bastion_root_live_unique ON public.hosts",
		"(role = 'bastion'::text)",
		"(status <> 'deleted'::text)",
	} {
		if !strings.Contains(string(base), required) {
			t.Fatalf("Bastion root lifecycle baseline is missing %q", required)
		}
	}
}

func TestBaselineUsesConnectorReportedBastionRelayEndpoint(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	base, err := os.ReadFile(filepath.Join(root, "migrations", "postgresql", "00001_argus_baseline.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"relay_address text DEFAULT ''::text NOT NULL",
		"relay_https_port integer DEFAULT 8445 NOT NULL",
		"relay_gateway_port integer DEFAULT 9445 NOT NULL",
		"relay_port_generation bigint DEFAULT 0 NOT NULL",
		"relay_error_code text DEFAULT ''::text NOT NULL",
	} {
		if !strings.Contains(string(base), required) {
			t.Fatalf("Bastion relay baseline is missing %q", required)
		}
	}
}
