package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// Keep the input to long natural-language conversations stable through real
// OTLP ingestion. A single point can age out while discovery/model rounds run.
// Missing-series and query-error scenarios still use their actual expressions.
func (a *App) startPlan6ModelTelemetry(ctx context.Context, env *E2EEnvironment) (func() error, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	resourceID := env.State.Values["m3_cluster_id"]
	emit := func() error {
		_, err := env.Kube.Exec(streamCtx, env.SystemNS, "app.kubernetes.io/name=argus-server", "plan6-telemetry-fixture", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--resource-id="+resourceID, "--plan6-alert-value=95")
		return err
	}
	if err := emit(); err != nil {
		cancel()
		return nil, err
	}
	started, last := time.Now().UTC(), time.Now().UTC()
	count := 1
	var failure error
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-streamCtx.Done():
				return
			case <-ticker.C:
				if err := emit(); err != nil {
					if streamCtx.Err() == nil {
						failure = fmt.Errorf("real Plan6 model telemetry emission failed: %w", err)
					}
					return
				}
				count++
				last = time.Now().UTC()
			}
		}
	}()
	return func() error {
		cancel()
		<-done
		facts := map[string]any{"real_otlp": true, "interval_seconds": 10, "value": 95, "emissions": count, "started_at": started, "last_emitted_at": last, "stopped_at": time.Now().UTC(), "namespace": env.SystemNS, "resource_id": resourceID, "passed": failure == nil}
		if failure != nil {
			facts["failure_reason"] = failure.Error()
		}
		raw, _ := json.MarshalIndent(facts, "", "  ")
		return errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan6-model-telemetry.json"), raw))
	}, nil
}
