package kubernetesreader

import (
	"context"
	"encoding/json"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

func (reader Reader) ListInventory(ctx context.Context, cluster db.KubernetesCluster, query kubernetesinventory.Query) (kubernetesinventory.Page, error) {
	if cluster.ConnectionMode == "direct" {
		client, cleanup, err := reader.directInventory(ctx, cluster)
		if err != nil {
			return kubernetesinventory.Page{}, err
		}
		defer cleanup()
		return client.List(ctx, query)
	}
	return reader.connectorInventory(ctx, cluster, query, false)
}
func (reader Reader) GetInventory(ctx context.Context, cluster db.KubernetesCluster, query kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	if cluster.ConnectionMode == "direct" {
		client, cleanup, err := reader.directInventory(ctx, cluster)
		if err != nil {
			return kubernetesinventory.Object{}, err
		}
		defer cleanup()
		return client.Get(ctx, query)
	}
	page, err := reader.connectorInventory(ctx, cluster, query, true)
	if err != nil {
		return kubernetesinventory.Object{}, err
	}
	if len(page.Items) != 1 {
		return kubernetesinventory.Object{}, kubernetesinventory.ErrIdentityChanged
	}
	return page.Items[0], nil
}
func (reader Reader) directInventory(ctx context.Context, cluster db.KubernetesCluster) (kubernetesinventory.Client, func(), error) {
	config, httpClient, cleanup, err := reader.directConfig(ctx, cluster)
	if err != nil {
		return kubernetesinventory.Client{}, cleanup, err
	}
	client, err := dynamic.NewForConfigAndClient(config, httpClient)
	if err != nil {
		cleanup()
		return kubernetesinventory.Client{}, func() {}, kubernetesinventory.ErrUnavailable
	}
	return kubernetesinventory.Client{Dynamic: client, MaxBytes: 1 << 20}, cleanup, nil
}
func (reader Reader) connectorInventory(ctx context.Context, cluster db.KubernetesCluster, query kubernetesinventory.Query, get bool) (kubernetesinventory.Page, error) {
	result := kubernetesinventory.Page{SchemaVersion: kubernetesinventory.SchemaVersion, Items: []kubernetesinventory.Object{}}
	definition, err := reader.InventoryDefinition(ctx, cluster, query.Type)
	if err != nil {
		return result, err
	}
	payload := &connectorv1.KubernetesResourceQuery{SchemaVersion: kubernetesinventory.SchemaVersion, ClusterId: cluster.ID.String(), ResourceType: query.Type, Namespace: query.Namespace, NamespaceUid: query.NamespaceUID, Name: query.Name, ObjectUid: query.UID, LabelSelector: query.Labels, Limit: uint32(query.Limit), ContinueToken: query.Continue, ResourceVersion: query.ResourceVersion, AllNamespaces: query.AllNamespaces, GetOnly: get, MaxResultBytes: 1 << 20}
	var response connectorv1.KubernetesResourceQueryResult
	if err := reader.connectorCommand(ctx, cluster, "kubernetes_resource_query", payload, &response); err != nil {
		return result, err
	}
	if response.SchemaVersion != kubernetesinventory.SchemaVersion || response.ObservedAt == nil || !response.ObservedAt.IsValid() {
		return result, kubernetesinventory.ErrProtocol
	}
	if err := kubernetesinventory.ErrorFromCode(response.ErrorCode); err != nil {
		return result, err
	}
	if response.Truncated || len(response.ResourcesJson) > 200 {
		return result, kubernetesinventory.ErrBudget
	}
	used := 0
	for _, raw := range response.ResourcesJson {
		used += len(raw)
		if used > 1<<20 {
			return result, kubernetesinventory.ErrBudget
		}
		var object kubernetesinventory.Object
		if json.Unmarshal(raw, &object) != nil || object.Definition != definition || query.Namespace != "" && (object.Namespace != query.Namespace || object.NamespaceUID != query.NamespaceUID) || query.UID != "" && object.UID != query.UID || query.Name != "" && object.Name != query.Name {
			return result, kubernetesinventory.ErrIdentityChanged
		}
		var data unstructured.Unstructured
		if data.UnmarshalJSON(object.Data) != nil || string(data.GetUID()) != object.UID || data.GetName() != object.Name || data.GetNamespace() != object.Namespace {
			return result, kubernetesinventory.ErrInvalid
		}
		projected, err := kubernetesinventory.Project(definition, data, object.NamespaceUID, response.ObservedAt.AsTime())
		if err != nil {
			return result, err
		}
		projected.Redacted = projected.Redacted || object.Redacted
		result.Items = append(result.Items, projected)
	}
	result.ResourceVersion = response.ResourceVersion
	result.Continue = response.ContinueToken
	result.Complete = result.Continue == ""
	result.ObservedAt = response.ObservedAt.AsTime()
	return result, nil
}
