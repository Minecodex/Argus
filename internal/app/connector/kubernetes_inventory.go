package connector

import (
	"context"
	"encoding/json"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/client-go/dynamic"
)

func executeKubernetesInventory(ctx context.Context, request *connectorv1.KubernetesResourceQuery, credential []byte) (*connectorv1.KubernetesResourceQueryResult, error) {
	result := &connectorv1.KubernetesResourceQueryResult{SchemaVersion: kubernetesinventory.SchemaVersion, ObservedAt: timestamppb.Now()}
	failure := func(err error) (*connectorv1.KubernetesResourceQueryResult, error) {
		result.ErrorCode = kubernetesinventory.ErrorCode(err)
		return result, nil
	}
	if request.SchemaVersion != kubernetesinventory.SchemaVersion {
		return failure(kubernetesinventory.ErrProtocol)
	}
	config, err := connectorKubeconfig(credential, "")
	if err != nil {
		return failure(kubernetesinventory.ErrUnavailable)
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return failure(kubernetesinventory.ErrUnavailable)
	}
	reader := kubernetesinventory.Client{Dynamic: client, MaxBytes: int(request.MaxResultBytes)}
	if request.DefinitionsOnly {
		result.ResultKind = "definitions"
		if request.WatchOnly || request.GetOnly {
			return failure(kubernetesinventory.ErrInvalid)
		}
		var catalog kubernetesinventory.TypeCatalog
		if request.ResourceType == "__inventory_types" {
			catalog, err = reader.Types(ctx)
		} else {
			var definition kubernetesinventory.Definition
			definition, err = reader.Definition(ctx, request.ResourceType)
			catalog = kubernetesinventory.TypeCatalog{Items: []kubernetesinventory.Definition{definition}, Complete: true}
		}
		if err != nil {
			return failure(err)
		}
		result.TypeCatalogComplete = catalog.Complete
		result.UnavailableReason = catalog.UnavailableReason
		for _, definition := range catalog.Items {
			raw, err := json.Marshal(definition)
			if err != nil {
				return failure(kubernetesinventory.ErrInvalid)
			}
			result.ResourcesJson = append(result.ResourcesJson, raw)
		}
		return result, nil
	}
	query := kubernetesinventory.Query{Type: request.ResourceType, Namespace: request.Namespace, NamespaceUID: request.NamespaceUid, Name: request.Name, UID: request.ObjectUid, Labels: request.LabelSelector, Limit: int64(request.Limit), Continue: request.ContinueToken, ResourceVersion: request.ResourceVersion, AllNamespaces: request.AllNamespaces}
	if request.WatchOnly {
		result.ResultKind = "watch"
		if request.GetOnly {
			return failure(kubernetesinventory.ErrInvalid)
		}
		batch, err := reader.Watch(ctx, query)
		if err != nil {
			return failure(err)
		}
		result.ResourceVersion = batch.ResourceVersion
		result.ObservedAt = timestamppb.New(batch.ObservedAt)
		result.More = batch.More
		for _, event := range batch.Events {
			raw, err := json.Marshal(event)
			if err != nil {
				return failure(kubernetesinventory.ErrInvalid)
			}
			result.ResourcesJson = append(result.ResourcesJson, raw)
		}
		return result, nil
	}
	var page kubernetesinventory.Page
	result.ResultKind = "page"
	if request.GetOnly {
		result.ResultKind = "object"
		object, e := reader.Get(ctx, query)
		if e != nil {
			return failure(e)
		}
		page = kubernetesinventory.Page{Items: []kubernetesinventory.Object{object}, ObservedAt: object.ObservedAt, ResourceVersion: object.ResourceVersion, Complete: true}
	} else {
		page, err = reader.List(ctx, query)
		if err != nil {
			return failure(err)
		}
	}
	result.ContinueToken = page.Continue
	result.ResourceVersion = page.ResourceVersion
	result.ObservedAt = timestamppb.New(page.ObservedAt)
	if page.ObservedAt.IsZero() {
		result.ObservedAt = timestamppb.New(time.Now().UTC())
	}
	for _, object := range page.Items {
		raw, err := json.Marshal(object)
		if err != nil {
			return failure(kubernetesinventory.ErrInvalid)
		}
		result.ResourcesJson = append(result.ResourcesJson, raw)
	}
	return result, nil
}
