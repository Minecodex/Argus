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

func TestGracefulShutdownPersistsFailuresAfterLastPeriodicSample(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		t.Fatal(err)
	}
	var failures atomic.Int64
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, deliveryFixture("final-process", terminalLogMetric(fmt.Sprint(failures.Load()))))
	})}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })
	path := filepath.Join(t.TempDir(), "ledger.json")
	extension := newIdentityExtension(Config{DeliveryEndpoint: "https://identity.invalid/v1/collector-delivery", DeliveryStateFile: path, CertificateFile: "missing-owned-certificate", PrivateKeyFile: "missing-owned-key"}, zap.NewNop())
	state, err := loadDeliveryState(path, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	// Reporting is intentionally unavailable; the observed facts must still be
	// persisted before that send and survive for the next process to report.
	_ = extension.observeDelivery(t.Context(), &state)
	if state.Report.TerminalFailures.LogRecords != 0 || state.Report.Sequence != 1 {
		t.Fatal(state.Report)
	}
	failures.Store(3)
	extension.cancel = func() {}
	close(extension.done)
	close(extension.deliveryDone)
	if err = extension.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	final, err := loadDeliveryState(path, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if final.Report.Sequence != 2 || final.Report.TerminalFailures.LogRecords != 3 || final.Report.LastFailureAt == nil || final.Report.InstanceID != state.Report.InstanceID {
		t.Fatal("graceful shutdown lost the final native failure or changed reporter identity", final.Report)
	}
}
