package toolgateway

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
)

func (t MonitoringTools) registerMonitoringHostForm(reg *mcp.Registry) error {
	properties := map[string]any{"integration": domainText(100), "collection_id": domainID(), "name": domainText(200), "endpoint": domainText(2048), "database": domainText(128)}
	return t.register(reg, "monitoring.collection.form", "monitoring.manage", "监控安全表单 / Monitoring secure form",
		"Ask the human to complete the trusted host form within this conversation. Specify an eligible integration and optionally public name/endpoint/database defaults. To replace credentials of an existing authorized collection, provide collection_id and its integration. Credentials, Key values and private draft references are never tool arguments or results. The host submits through the same portal API and requires normal change confirmation; requesting the form creates no monitoring configuration. Read the collection and its actual samples after the human confirms.", objectSchema(properties, "integration"), func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			if a.SubjectType != "user" {
				return nil, monitoring.ErrDenied
			}
			var in struct {
				Integration, Name, Endpoint, Database string
				CollectionID                          uuid.UUID `json:"collection_id"`
			}
			value, err := json.Marshal(c.Input)
			if err != nil || json.Unmarshal(value, &in) != nil {
				return nil, monitoring.ErrInvalid
			}
			entries, err := t.Monitoring.Integrations(ctx, a)
			if err != nil {
				return nil, err
			}
			eligible := false
			for _, entry := range entries {
				if entry.Key == in.Integration && entry.AdapterReady && entry.Selection == "candidate" {
					eligible = true
				}
			}
			if !eligible {
				return nil, monitoring.ErrUnsupported
			}
			result := map[string]any{"schema_version": "argus.monitoring_host_form/v1", "status": "requires_user_input", "integration": in.Integration}
			if in.CollectionID != uuid.Nil {
				collection, err := t.Monitoring.Collection(ctx, a, in.CollectionID)
				if err != nil {
					return nil, err
				}
				if collection.Integration != in.Integration || in.Name != "" || in.Endpoint != "" || in.Database != "" {
					return nil, monitoring.ErrInvalid
				}
				result["collection_id"], result["target"] = collection.ID, collection.Target
			} else {
				if in.Endpoint != "" {
					identity, _ := json.Marshal(monitoringcatalog.TargetIdentity{Endpoint: in.Endpoint, Database: in.Database})
					if _, _, err := monitoringcatalog.Parse(in.Integration, identity, []byte(`{}`)); err != nil {
						return nil, monitoring.ErrInvalid
					}
				}
				result["prefill"] = map[string]string{"name": in.Name, "endpoint": in.Endpoint, "database": in.Database}
			}
			return result, nil
		})
}
