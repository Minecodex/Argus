package kubernetesreader

import (
	"context"
	"encoding/json"
	"slices"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (reader Reader) WatchInventory(ctx context.Context, cluster db.KubernetesCluster, query kubernetesinventory.Query) (kubernetesinventory.WatchBatch, error) {
	if cluster.ConnectionMode == "direct" {
		client, cleanup, err := reader.directInventory(ctx, cluster)
		if err != nil {
			return kubernetesinventory.WatchBatch{}, err
		}
		defer cleanup()
		return client.Watch(ctx, query)
	}
	definition, err := reader.InventoryDefinition(ctx, cluster, query.Type)
	if err != nil {
		return kubernetesinventory.WatchBatch{}, err
	}
	request := &connectorv1.KubernetesResourceQuery{SchemaVersion: kubernetesinventory.SchemaVersion, ClusterId: cluster.ID.String(), ResourceType: query.Type, Namespace: query.Namespace, NamespaceUid: query.NamespaceUID, Name: query.Name, LabelSelector: query.Labels, Limit: uint32(query.Limit), ResourceVersion: query.ResourceVersion, AllNamespaces: query.AllNamespaces, WatchOnly: true, MaxResultBytes: 1 << 20}
	var response connectorv1.KubernetesResourceQueryResult
	if err := reader.connectorCommand(ctx, cluster, "kubernetes_resource_query", request, &response); err != nil {
		return kubernetesinventory.WatchBatch{}, err
	}
	if response.SchemaVersion != kubernetesinventory.SchemaVersion || response.ResultKind != "watch" || response.ObservedAt == nil || !response.ObservedAt.IsValid() {
		return kubernetesinventory.WatchBatch{}, kubernetesinventory.ErrProtocol
	}
	if err := kubernetesinventory.ErrorFromCode(response.ErrorCode); err != nil {
		return kubernetesinventory.WatchBatch{}, err
	}
	if response.Truncated || len(response.ResourcesJson) > 200 {
		return kubernetesinventory.WatchBatch{}, kubernetesinventory.ErrBudget
	}
	out := kubernetesinventory.WatchBatch{SchemaVersion: kubernetesinventory.SchemaVersion, Events: []kubernetesinventory.Event{}, ResourceVersion: response.ResourceVersion, ObservedAt: response.ObservedAt.AsTime(), More: response.More}
	used := 0
	for _, raw := range response.ResourcesJson {
		used += len(raw)
		if used > 1<<20 {
			return kubernetesinventory.WatchBatch{}, kubernetesinventory.ErrBudget
		}
		var event kubernetesinventory.Event
		if json.Unmarshal(raw, &event) != nil || !slices.Contains([]string{"ADDED", "MODIFIED", "DELETED"}, event.Type) || event.Object.Definition != definition {
			return kubernetesinventory.WatchBatch{}, kubernetesinventory.ErrProtocol
		}
		object := event.Object
		if query.Namespace != "" && (object.Namespace != query.Namespace || object.NamespaceUID != query.NamespaceUID) || query.Name != "" && object.Name != query.Name {
			return kubernetesinventory.WatchBatch{}, kubernetesinventory.ErrIdentityChanged
		}
		var data unstructured.Unstructured
		if data.UnmarshalJSON(object.Data) != nil || string(data.GetUID()) != object.UID || data.GetName() != object.Name || data.GetNamespace() != object.Namespace {
			return kubernetesinventory.WatchBatch{}, kubernetesinventory.ErrInvalid
		}
		projected, err := kubernetesinventory.Project(definition, data, object.NamespaceUID, out.ObservedAt)
		if err != nil {
			return kubernetesinventory.WatchBatch{}, err
		}
		projected.Redacted = projected.Redacted || object.Redacted
		event.Object = projected
		out.Events = append(out.Events, event)
	}
	return out, nil
}
