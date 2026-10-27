package argusidentity

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestDeliveryObservationDoesNotAccumulateIdleConnections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		t.Fatal("owned loopback observation endpoint unavailable", err)
	}
	var open atomic.Int64
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, deliveryFixture("owned-runtime", terminalLogMetric("0")))
	}), ConnState: func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			open.Add(1)
		}
		if state == http.StateClosed {
			open.Add(-1)
		}
	}}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })
	path := filepath.Join(t.TempDir(), "delivery.json")
	extension := newIdentityExtension(Config{DeliveryStateFile: path, CertificateFile: "missing-owned-fixture-certificate", PrivateKeyFile: "missing-owned-fixture-key"}, zap.NewNop())
	state, err := loadDeliveryState(path, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		_ = extension.observeDelivery(context.Background(), &state)
	}
	if state.Report.Sequence != 100 {
		t.Fatal("observations did not reach the durable ledger", state.Report.Sequence)
	}
	deadline := time.Now().Add(time.Second)
	for open.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if open.Load() != 0 {
		t.Fatal("periodic observation retained idle connections", open.Load())
	}
}

func TestDeliveryConfigRejectsUntrustedControlReportDestinations(t *testing.T) {
	base := *(createDefaultConfig().(*Config))
	base.CollectorID = "owned-collector"
	base.EnrollmentEndpoint = "https://identity.argus.test/v1/identity/enroll"
	base.RotationEndpoint = "https://identity.argus.test/v1/identity/rotate"
	base.TrustBundleEndpoint = "https://identity.argus.test/v1/identity/trust-bundle"
	base.DeliveryEndpoint = "https://identity.argus.test/v1/collector-delivery"
	base.DeliveryStateFile = filepath.Join(t.TempDir(), "delivery.json")
	if err := base.Validate(); err != nil {
		t.Fatal("valid managed destination rejected", err)
	}
	for _, endpoint := range []string{"http://identity.argus.test/v1/collector-delivery", "https://other.argus.test/v1/collector-delivery", "https://private@identity.argus.test/v1/collector-delivery", "https://identity.argus.test/v1/collector-delivery?forward=private", "https://identity.argus.test/v1/collector-delivery#private", "https://identity.argus.test/arbitrary"} {
		config := base
		config.DeliveryEndpoint = endpoint
		if err := config.Validate(); err == nil {
			t.Fatal("untrusted delivery destination accepted")
		}
	}
}
