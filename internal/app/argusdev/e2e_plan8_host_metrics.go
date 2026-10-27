package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func (a *App) plan8FreshHostSample(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	host, err := uuid.Parse(h.HostID)
	if err != nil {
		return err
	}
	collector, err := uuid.Parse(h.CollectorID)
	if err != nil {
		return err
	}
	after := time.Now().UTC()
	_, err = a.plan8ServiceEffective(ctx, env, builtincollection.StableID("host", host, "host_metrics"), collector, true, &after)
	return err
}

func (a *App) enablePlan8HostMetrics(ctx context.Context, env *E2EEnvironment, host string) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	items, err := client.JSONArray(ctx, "plan8-host-builtins", "enterprise", http.MethodGet, "/monitoring/builtins?target_type=host&target_id="+host, http.StatusOK, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	for _, item := range items {
		body, err := json.Marshal(item)
		if err != nil {
			return err
		}
		var entry monitoring.Builtin
		if err = json.Unmarshal(body, &entry); err != nil {
			return err
		}
		if entry.Definition.Key != "host_metrics" {
			continue
		}
		if !entry.Available {
			return fmt.Errorf("host metrics capability unavailable: %s", entry.Reason)
		}
		version := int64(0)
		if entry.Collection != nil {
			version = entry.Collection.Version
		}
		preview, err := client.JSON(ctx, "plan8-host-metrics-preview", "enterprise", http.MethodPost, "/monitoring/builtins/preview", http.StatusCreated, map[string]any{"target": map[string]string{"kind": "host", "id": host}, "key": "host_metrics", "enabled": true, "expected_version": version, "config": map[string]string{"interval": "5s"}}, enterpriseHeaders(env, "plan8-host-metrics-preview"))
		if err != nil {
			return err
		}
		ref, err := stringField(preview, "action_ref")
		if err != nil {
			return err
		}
		if _, err = a.confirmPendingAction(ctx, env, "plan8-host-metrics", ref); err != nil {
			return err
		}
		_, err = a.waitP4HostCollector(ctx, env, host, "converged")
		return err
	}
	return fmt.Errorf("host metrics definition was not returned")
}
