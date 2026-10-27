package toolgateway

import (
	"context"

	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
)

func (t MonitoringTools) registerDrainCancellation(reg *mcp.Registry) error {
	schema := objectSchema(map[string]any{"cluster_id": domainID(), "id": domainID(), "expected_version": map[string]any{"type": "integer", "minimum": 1}}, "cluster_id", "id", "expected_version")
	return t.register(reg, "kubernetes.drain.cancel.preview", "kubernetes.node.drain", "取消节点排空预览 / Preview cancelling node drain",
		"Prepare cancellation of a drain operation using its current observed operation version. Requires explicit Cluster scope and node manage/drain permissions. Confirmation stops future batches; already dispatched evictions may finish and the Node stays cordoned. This does not restore Pods or uncordon the Node.", schema,
		func(ctx context.Context, a authorization.Actor, c mcp.Call) (any, error) {
			in, err := domainInput[kubernetesworkbench.DrainCancelInput](c)
			if err != nil {
				return nil, err
			}
			pending, err := t.Kubernetes.PreviewDrainCancel(ctx, a, in, idempotency(c))
			return pendingProjection(pending), err
		})
}
