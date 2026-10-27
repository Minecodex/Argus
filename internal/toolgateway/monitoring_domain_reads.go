package toolgateway

import (
	"context"

	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func domainListSchema(fields ...string) map[string]any {
	props := map[string]any{"search": domainText(200), "cursor": domainText(32768), "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}}
	extra := map[string]any{"status": map[string]any{"type": "string", "enum": []string{"active", "archived"}}, "integration": domainText(100), "application_ids": map[string]any{"type": "array", "maxItems": 100, "items": domainID()}}
	extra["desired_enabled"] = map[string]any{"type": "boolean", "description": "Omit for all enablement states; false selects disabled collections."}
	extra["apply_status"] = map[string]any{"type": "string", "enum": []string{"pending", "applying", "effective", "failed", "unknown"}}
	extra["health"] = map[string]any{"type": "string", "enum": []string{"unknown", "healthy", "degraded", "failed", "stopped"}}
	extra["freshness"] = map[string]any{"type": "string", "enum": []string{"no_data", "fresh", "stale"}}
	for _, field := range fields {
		props[field] = extra[field]
	}
	return objectSchema(props)
}

func (t MonitoringTools) registerMonitoring(reg *mcp.Registry) error {
	for _, item := range []struct {
		id, permission, title string
		run                   func(context.Context, authorization.Actor, monitoring.ListInput) (any, error)
	}{
		{"application.list", "application.read", "应用列表 / Applications", func(ctx context.Context, a authorization.Actor, in monitoring.ListInput) (any, error) {
			return t.Monitoring.Applications(ctx, a, in)
		}},
		{"application.service.list", "application.read", "应用服务列表 / Application services", func(ctx context.Context, a authorization.Actor, in monitoring.ListInput) (any, error) {
			return t.Monitoring.Services(ctx, a, in)
		}},
		{"monitoring.target.list", "monitoring.read", "监控目标列表 / Monitoring targets", func(ctx context.Context, a authorization.Actor, in monitoring.ListInput) (any, error) {
			return t.Monitoring.Targets(ctx, a, in)
		}},
		{"monitoring.collection.list", "monitoring.read", "采集实例列表 / Collection instances", func(ctx context.Context, a authorization.Actor, in monitoring.ListInput) (any, error) {
			return t.Monitoring.Collections(ctx, a, in)
		}},
		{"monitoring.collector.choices", "collector.use", "可用采集器 / Collector choices", func(ctx context.Context, a authorization.Actor, in monitoring.ListInput) (any, error) {
			return t.Monitoring.CollectorChoices(ctx, a, in)
		}},
	} {
		item := item
		fields := []string{"integration"}
		if item.id == "application.list" {
			fields = []string{"status"}
		} else if item.id == "application.service.list" {
			fields = []string{"application_ids"}
		} else if item.id == "monitoring.collection.list" {
			fields = []string{"integration", "desired_enabled", "apply_status", "health", "freshness"}
		}
		if err := t.register(reg, item.id, item.permission, item.title, "Search a bounded authoritative directory. Follow next_cursor to obtain subsequent pages; a page is not the complete inventory. Application environment is inherited, and relationships do not grant resource access.", domainListSchema(fields...), func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			in, e := domainInput[monitoring.ListInput](c)
			if e != nil {
				return nil, e
			}
			return item.run(ctx, a, in)
		}); err != nil {
			return err
		}
	}
	for _, item := range []struct {
		id, permission, title string
		run                   func(context.Context, authorization.Actor, mcp.Call) (any, error)
	}{
		{"application.get", "application.read", "应用详情 / Application details", func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			id, e := callID(c, "id")
			if e != nil {
				return nil, e
			}
			return t.Monitoring.Application(ctx, a, id)
		}},
		{"application.endpoint.connection", "application.read", "SDK 接入配置 / SDK connection configuration", func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			id, e := callID(c, "id")
			if e != nil {
				return nil, e
			}
			return t.Monitoring.IngestConnection(ctx, a, id)
		}},
		{"monitoring.collection.get", "monitoring.read", "采集详情 / Collection details", func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			id, e := callID(c, "id")
			if e != nil {
				return nil, e
			}
			return t.Monitoring.Collection(ctx, a, id)
		}},
	} {
		if err := t.register(reg, item.id, item.permission, item.title, "Read an authorized object's current public state. Desired enablement, effective revision, health and freshness are separate fields.", idSchema("id"), item.run); err != nil {
			return err
		}
	}
	if err := t.register(reg, "monitoring.integration.list", "monitoring.read", "监控能力目录 / Integration catalog", "Inspect embedded components and their admission status. Candidate/configuration availability is not a validated support claim. Deferred entries cannot be enabled.", emptyObjectSchema(), func(ctx context.Context, a authorization.Actor, _ mcp.Call) (any, error) {
		items, e := t.Monitoring.Integrations(ctx, a)
		return map[string]any{"items": items}, e
	}); err != nil {
		return err
	}
	if err := t.registerBuiltins(reg); err != nil {
		return err
	}
	for _, item := range []struct{ id, field, title string }{
		{"application.endpoint.list", "application_id", "应用接入点 / Application endpoints"},
		{"application.key.list", "endpoint_id", "接入 Key 元数据 / Ingest Key metadata"},
	} {
		item := item
		schema := domainListSchema()
		schema["properties"].(map[string]any)[item.field] = domainID()
		schema["required"] = []string{item.field}
		if err := t.register(reg, item.id, "application.read", item.title, "Read only public endpoint/Key metadata. Key values are never returned. SDK service registration occurs after authenticated ingestion; precreating a service is optional.", schema, func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			id, e := callID(c, item.field)
			if e != nil {
				return nil, e
			}
			in, e := domainInput[monitoring.ListInput](c)
			if e != nil {
				return nil, e
			}
			if item.field == "application_id" {
				return t.Monitoring.IngestEndpoints(ctx, a, id, in)
			}
			return t.Monitoring.IngestKeys(ctx, a, id, in)
		}); err != nil {
			return err
		}
	}
	return t.registerMonitoringPreviews(reg)
}

func (t MonitoringTools) registerBuiltins(reg *mcp.Registry) error {
	doc, err := nativePreviewDocument()
	if err != nil {
		return err
	}
	schema, err := previewSchema(doc, "MonitoringBuiltinInput", "")
	if err != nil {
		return err
	}
	target := schema["properties"].(map[string]any)["target"]
	if err = t.register(reg, "monitoring.builtin.list", "telemetry.collector.read", "自身监控能力 / Own-resource capabilities", "Inspect fixed Host/Cluster collection instances, availability and separate desired/effective/health states. This does not create a new resource target.", objectSchema(map[string]any{"target": target}, "target"), func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
		in, e := domainInput[monitoring.BuiltinInput](c)
		if e != nil {
			return nil, e
		}
		items, e := t.Monitoring.Builtins(ctx, a, in.Target)
		return map[string]any{"items": items}, e
	}); err != nil {
		return err
	}
	return t.register(reg, "monitoring.builtin.change.preview", "telemetry.collector.manage", "自身监控启停预览 / Preview own-resource collection", "Enable or disable a stable fixed capability with its observed version. Missing endpoints, permissions or metrics remain unavailable; a successful configuration is not proof of fresh samples.", schema, func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
		in, e := domainInput[monitoring.BuiltinInput](c)
		if e != nil {
			return nil, e
		}
		pending, e := t.Monitoring.PreviewBuiltin(ctx, a, in, idempotency(c))
		return pendingProjection(pending), e
	})
}

func (t MonitoringTools) registerMonitoringPreviews(reg *mcp.Registry) error {
	doc, err := nativePreviewDocument()
	if err != nil {
		return err
	}
	for _, item := range []struct {
		id, permission, title string
		kinds                 []string
	}{
		{"application.change.preview", "application.manage", "应用配置变更预览 / Preview application changes", []string{"application", "service", "binding", "endpoint", "ingest_key"}},
		{"monitoring.collection.change.preview", "monitoring.manage", "采集实例变更预览 / Preview collection changes", []string{"collection"}},
	} {
		item := item
		schema, e := previewSchema(doc, "MonitoringMutation", "")
		if e != nil {
			return e
		}
		props := schema["properties"].(map[string]any)
		delete(props, "credentials")
		props["kind"] = map[string]any{"type": "string", "enum": item.kinds}
		if collection, ok := props["collection"].(map[string]any); ok {
			if cp, ok := collection["properties"].(map[string]any); ok {
				cp["credential_update"] = map[string]any{"type": "string", "enum": []string{"keep", "remove"}}
			}
		}
		if err = t.register(reg, item.id, item.permission, item.title, "Prepare an immutable change using the same domain service as the portal. Confirmation is required. Environment cannot be edited; new production/test applications are distinct. Credential replacement requires the trusted portal form. Creating/rotating an SDK Key delivers its value only through the user's one-time result.", schema, func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			if e := rejectPrivateDomainInput(c.Input); e != nil {
				return nil, e
			}
			in, e := domainInput[monitoring.Mutation](c)
			if e != nil {
				return nil, e
			}
			pending, e := t.Monitoring.Preview(ctx, a, in, idempotency(c))
			return pendingProjection(pending), e
		}); err != nil {
			return err
		}
	}
	return nil
}
