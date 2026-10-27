package toolgateway

import (
	"context"

	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
)

func (t MonitoringTools) registerHelm(reg *mcp.Registry) error {
	for _, history := range []bool{false, true} {
		id, title := "kubernetes.helm.list", "Helm 发布列表 / List Helm releases"
		properties := map[string]any{"cluster_id": domainID(), "namespace_id": domainID()}
		required := []string{"cluster_id", "namespace_id"}
		if history {
			id, title = "kubernetes.helm.history", "Helm 版本历史 / Helm revision history"
			properties["name"] = domainText(53)
			required = append(required, "name")
		}
		if err := t.register(reg, id, "kubernetes.read", title, "Read safe Helm release metadata in one explicitly selected authorized Namespace. Chart contents, values, manifests and hook logs are never returned. Use the trusted portal to install or upgrade a packaged chart.", objectSchema(properties, required...), func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			cluster, e := callID(c, "cluster_id")
			if e != nil {
				return nil, e
			}
			namespace, e := callID(c, "namespace_id")
			if e != nil {
				return nil, e
			}
			rows, e := t.Kubernetes.HelmReleases(ctx, a, cluster, namespace, stringValue(c.Input, "name"), history)
			return map[string]any{"items": rows}, e
		}); err != nil {
			return err
		}
	}
	properties := map[string]any{"cluster_id": domainID(), "namespace_id": domainID(), "name": domainText(53), "operation": map[string]any{"type": "string", "enum": []string{"rollback", "uninstall"}}, "expected_revision": map[string]any{"type": "integer", "minimum": 1}, "rollback_revision": map[string]any{"type": "integer", "minimum": 1}, "timeout_seconds": map[string]any{"type": "integer", "minimum": 10, "maximum": 240}, "reason": domainText(500)}
	return t.register(reg, "kubernetes.helm.change.preview", "kubernetes.helm.manage", "Helm 回滚与卸载预览 / Preview Helm rollback or uninstall", "Prepare an immutable rollback or uninstall from observed revision metadata. Every rendered object requires current type and Namespace UID permissions; Helm manage is insufficient by itself. Human confirmation is required. Uninstall keeps release history and retained objects. Install and upgrade require protected chart/values entry in the trusted portal, unavailable in this model tool.", objectSchema(properties, "cluster_id", "namespace_id", "name", "operation", "expected_revision", "reason"), func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
		cluster, e := callID(c, "cluster_id")
		if e != nil {
			return nil, e
		}
		input, e := domainInput[kubernetesworkbench.HelmInput](c)
		if e != nil {
			return nil, e
		}
		pending, e := t.Kubernetes.PreviewHelm(ctx, a, cluster, input, nil, nil, idempotency(c))
		return pendingProjection(pending), e
	})
}
