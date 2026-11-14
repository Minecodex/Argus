package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type auditedDefinitionBackend struct {
	*mutationFixtureBackend
	store           *postgres.Store
	actor           Actor
	definition      kubernetesinventory.Definition
	definitionCalls int
	afterPreview    func()
}

func (b *auditedDefinitionBackend) InventoryDefinition(ctx context.Context, _ db.KubernetesCluster, key string) (kubernetesinventory.Definition, error) {
	b.definitionCalls++
	if key != b.definition.Key {
		return kubernetesinventory.Definition{}, kubernetesinventory.ErrIdentityChanged
	}
	// Real Direct metadata lookup issues/consumes a lease, advancing the audit
	// chain in its own transaction. Reproduce that side effect without a network.
	err := b.store.InTx(ctx, func(q *db.Queries) error {
		_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: b.actor.EnterpriseID, Valid: true},
			ActorType: "system", ActorID: "custom-definition-reader", Action: "test.definition.lease", ResourceType: "kubernetes_cluster", Result: "success"})
		return err
	})
	return b.definition, err
}

func (b *auditedDefinitionBackend) InventoryTypes(context.Context, db.KubernetesCluster) (kubernetesinventory.TypeCatalog, error) {
	return kubernetesinventory.TypeCatalog{Items: []kubernetesinventory.Definition{b.definition}, Complete: true}, nil
}

func (b *auditedDefinitionBackend) MutateInventory(_ context.Context, _ db.KubernetesCluster, input kubernetesinventory.Mutation, dry bool) (kubernetesinventory.MutationResult, error) {
	b.previews++
	if !dry {
		b.writes++
	}
	data, _ := json.Marshal(map[string]any{"apiVersion": "example.test/v1", "kind": "Widget", "metadata": map[string]any{"name": input.Query.Name, "namespace": input.Query.Namespace}, "spec": map[string]any{"message": "actual preview"}})
	if b.afterPreview != nil {
		b.afterPreview()
	}
	return kubernetesinventory.MutationResult{DryRun: dry, Accepted: !dry, Digest: "bound-digest", After: &kubernetesinventory.Object{
		Definition: b.definition, Name: input.Query.Name, Namespace: input.Query.Namespace, NamespaceUID: input.Query.NamespaceUID, Data: data}}, nil
}

func customPreviewFixture(t *testing.T) (*mutationFixture, *auditedDefinitionBackend, MutationInput) {
	t.Helper()
	f := newMutationFixture(t)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'kubernetes.custom_resource.manage' FROM role_bindings WHERE subject_id=$1", f.actor.SubjectID)
	key := "custom/example.test/v1/widgets@original-crd-uid"
	b := &auditedDefinitionBackend{mutationFixtureBackend: f.backend, store: f.service.Store, actor: f.actor, definition: kubernetesinventory.Definition{
		Key: key, Group: "example.test", Version: "v1", Resource: "widgets", Kind: "Widget", Namespaced: true, Category: "extension", ManagePermission: "kubernetes.custom_resource.manage"}}
	f.service.Backend = b
	in := MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Type: key, Operation: "create", Name: "sample", Object: json.RawMessage(`{"apiVersion":"example.test/v1","kind":"Widget","metadata":{"name":"sample","namespace":"orders"},"spec":{"message":"actual preview"}}`)}
	return f, b, in
}

func TestPostgresCustomPreviewResolvesAuditedMetadataOutsideTransaction(t *testing.T) {
	f, b, in := customPreviewFixture(t)
	action, err := f.service.PreviewMutation(t.Context(), f.actor, in, uuid.NewString())
	if err != nil {
		t.Fatalf("audited definition lookup self-invalidated preview: %v", err)
	}
	if action.ActionRef == "" || b.definitionCalls != 1 || b.previews != 1 || b.writes != 0 {
		t.Fatalf("unexpected preview effects: definitions=%d dry-runs=%d writes=%d", b.definitionCalls, b.previews, b.writes)
	}
}

func TestPostgresCustomPreviewRechecksScopeAfterNativeValidation(t *testing.T) {
	f, b, in := customPreviewFixture(t)
	b.afterPreview = func() {
		f.exec("UPDATE kubernetes_clusters SET resource_version=resource_version+1 WHERE id=$1", f.cluster)
	}
	_, err := f.service.PreviewMutation(t.Context(), f.actor, in, uuid.NewString())
	if err != resource.ErrActionInvalidated {
		t.Fatalf("changed cluster version accepted: %v", err)
	}
	if b.definitionCalls != 1 || b.writes != 0 {
		t.Fatal("scope rejection performed another native lookup or write")
	}
}

func TestPostgresCustomConfirmationRetainsIdentityAndRejectsRecreatedTypeBeforeWrite(t *testing.T) {
	f, b, in := customPreviewFixture(t)
	id, _ := f.queueInput(in)
	if b.definitionCalls != 1 {
		t.Fatalf("confirmation opened audited native metadata transactions: %d", b.definitionCalls)
	}
	b.definition.Key = "custom/example.test/v1/widgets@replacement-crd-uid"
	if err := (Worker{Service: f.service}).execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	row := f.operation(id)
	if row.Status != "failed" || row.ErrorCode != kubernetesinventory.ErrIdentityChanged.Error() || b.writes != 0 {
		t.Fatalf("replacement type received an old approval: status=%s code=%s writes=%d", row.Status, row.ErrorCode, b.writes)
	}
}
