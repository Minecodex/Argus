package toolgateway

import (
	"context"

	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func (t MonitoringTools) registerWorkbench(reg *mcp.Registry) error {
	selection := objectSchema(map[string]any{"mode": map[string]any{"type": "string", "enum": []string{"all_authorized", "explicit"}}, "ids": map[string]any{"type": "array", "maxItems": 200, "items": domainID()}}, "mode", "ids")
	list := objectSchema(map[string]any{"cluster_id": domainID(), "type": domainText(300), "namespaces": selection, "name": domainText(253), "labels": domainText(4096), "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 200}, "cursor": domainText(32768)}, "cluster_id", "type", "namespaces")
	get := objectSchema(map[string]any{"cluster_id": domainID(), "namespace_id": domainID(), "type": domainText(300), "name": domainText(253), "uid": domainText(128)}, "cluster_id", "type", "name", "uid")
	for _, item := range []struct {
		id, title string
		schema    map[string]any
		run       func(context.Context, authorization.Actor, mcp.Call) (any, error)
	}{
		{"kubernetes.workbench.types", "K8s 类型目录 / Kubernetes types", idSchema("cluster_id"), func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			id, e := callID(c, "cluster_id")
			if e != nil {
				return nil, e
			}
			return t.Kubernetes.Types(ctx, a, id)
		}},
		{"kubernetes.object.list", "K8s 对象列表 / Kubernetes objects", list, func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			id, e := callID(c, "cluster_id")
			if e != nil {
				return nil, e
			}
			in, e := domainInput[kubernetesworkbench.ListInput](c)
			if e != nil {
				return nil, e
			}
			return t.Kubernetes.List(ctx, a, id, in)
		}},
		{"kubernetes.object.get", "K8s 对象详情 / Kubernetes object details", get, func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			id, e := callID(c, "cluster_id")
			if e != nil {
				return nil, e
			}
			in, e := domainInput[kubernetesworkbench.ObjectInput](c)
			if e != nil {
				return nil, e
			}
			return t.Kubernetes.Get(ctx, a, id, in)
		}},
		{"kubernetes.operation.list", "K8s 操作记录 / Kubernetes operations", objectSchema(map[string]any{"cluster_id": domainID(), "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}, "cursor": domainText(32768)}, "cluster_id"), func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			id, e := callID(c, "cluster_id")
			if e != nil {
				return nil, e
			}
			return t.Kubernetes.ListOperations(ctx, a, id, int(intValue(c.Input, "limit")), stringValue(c.Input, "cursor"))
		}},
		{"kubernetes.operation.get", "K8s 操作状态 / Kubernetes operation status", objectSchema(map[string]any{"cluster_id": domainID(), "id": domainID()}, "cluster_id", "id"), func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			cluster, e := callID(c, "cluster_id")
			if e != nil {
				return nil, e
			}
			id, e := callID(c, "id")
			if e != nil {
				return nil, e
			}
			return t.Kubernetes.GetOperation(ctx, a, cluster, id)
		}},
	} {
		if err := t.register(reg, item.id, "kubernetes.read", item.title, "Use the shared Kubernetes workbench with current Cluster/Namespace UID authorization. Discover supported types first. Server cursors and explicit empty namespace selections preserve scope; object details require the observed UID. Secret values are projected out.", item.schema, item.run); err != nil {
			return err
		}
	}
	doc, err := nativePreviewDocument()
	if err != nil {
		return err
	}
	schema, err := previewSchema(doc, "KubernetesMutationInput", "cluster_id")
	if err != nil {
		return err
	}
	delete(schema["properties"].(map[string]any), "secret_values")
	return t.register(reg, "kubernetes.object.change.preview", "kubernetes.read", "K8s 对象变更预览 / Preview Kubernetes object changes", "Prepare a typed mutation, workload rollback, CronJob manual Job, or node drain using observed UID/resourceVersion. Drain additionally requires kubernetes.node.drain and Cluster scope; its preview freezes affected Pod UIDs, namespaces and storage, respects PDBs, and blocks local data or unmanaged Pods. Actual action-specific permissions are checked by the domain service, including ServiceAccount and escalation privileges. Secret writes require the trusted portal; this tool cannot submit Secret values. Confirm does not accept replacement parameters. An accepted API mutation is not proof that a workload is ready.", schema, func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
		if err := rejectPrivateDomainInput(c.Input); err != nil {
			return nil, err
		}
		in, err := domainInput[kubernetesworkbench.MutationInput](c)
		if err != nil {
			return nil, err
		}
		if in.Type == "secret" {
			return nil, monitoring.ErrCredential
		}
		pending, err := t.Kubernetes.PreviewMutation(ctx, a, in, idempotency(c))
		return pendingProjection(pending), err
	})
}
