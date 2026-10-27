package secret

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var ErrKubernetesPrivateInput = errors.New("Kubernetes private input unavailable")

func kubernetesInputAAD(enterprise, id, operation uuid.UUID) []byte {
	return []byte("argus.kubernetes_private_input/v1\x00" + enterprise.String() + "\x00" + id.String() + "\x00" + operation.String())
}
func kubernetesMutationHash(input kubernetesinventory.Mutation) ([]byte, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var canonical any
	if err = decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	raw, err = json.Marshal(canonical)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}
func authorizeKubernetesPrivateInput(ctx context.Context, q *db.Queries, actor authorization.Actor, cluster, namespace uuid.UUID, namespaceUID string) error {
	if err := authorization.Authorize(ctx, q, actor, "kubernetes.configuration.manage"); err != nil {
		return err
	}
	if err := authorization.RequireResource(ctx, q, actor, "kubernetes_namespace", namespace); err != nil {
		return err
	}
	ns, err := q.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: actor.EnterpriseID, ID: namespace})
	if err != nil {
		return err
	}
	if ns.ClusterID != cluster || ns.NamespaceUid != namespaceUID || ns.Status != "active" {
		return ErrKubernetesPrivateInput
	}
	return nil
}

func (s Service) StageKubernetesInput(ctx context.Context, actor authorization.Actor, cluster, namespace uuid.UUID, input kubernetesinventory.Mutation, values []byte) error {
	id, e1 := uuid.Parse(input.PrivateInputID)
	operation, e2 := uuid.Parse(input.OperationRef)
	if e1 != nil || e2 != nil || input.Query.Type != "secret" || input.Operation != "create" && input.Operation != "patch" || len(values) == 0 || len(values) > 64<<10 {
		return ErrKubernetesPrivateInput
	}
	if err := authorizeKubernetesPrivateInput(ctx, s.Store.Queries, actor, cluster, namespace, input.Query.NamespaceUID); err != nil {
		return err
	}
	// Random padding prevents the envelope's integrity hash from becoming an
	// offline dictionary for low-entropy values. The existing keyring owns DEK/KEK.
	padding := make([]byte, 32)
	if _, err := rand.Read(padding); err != nil {
		return err
	}
	material := struct {
		Salt   string          `json:"salt"`
		Values json.RawMessage `json:"values"`
	}{base64.RawURLEncoding.EncodeToString(padding), values}
	plaintext, err := json.Marshal(material)
	if err != nil {
		return ErrKubernetesPrivateInput
	}
	defer clear(plaintext)
	envelope, err := s.Keyring.EncryptContext(ctx, plaintext, kubernetesInputAAD(actor.EnterpriseID, id, operation))
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	hash, err := kubernetesMutationHash(input)
	if err != nil {
		return err
	}
	return s.Store.InTx(ctx, func(q *db.Queries) error {
		if err := authorizeKubernetesPrivateInput(ctx, q, actor, cluster, namespace, input.Query.NamespaceUID); err != nil {
			return err
		}
		if err := q.CreateKubernetesPrivateInput(ctx, db.CreateKubernetesPrivateInputParams{ID: id, EnterpriseID: actor.EnterpriseID, OperationID: operation, ClusterID: cluster, NamespaceID: namespace, NamespaceUid: input.Query.NamespaceUID, ActorID: actor.SubjectID, ActorType: actor.SubjectType, AuthorizationVersion: actor.AuthorizationVersion, MutationHash: hash, Envelope: encoded}); err != nil {
			return err
		}
		actorType := "enterprise_user"
		if actor.SubjectType == "service_account" {
			actorType = "service_account"
		}
		_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: actor.EnterpriseID, Valid: true}, ActorType: actorType, ActorID: actor.SubjectID.String(), Action: "kubernetes.private_input.stage", ResourceType: "kubernetes_cluster", ResourceID: cluster.String(), Result: "success", Details: map[string]any{"operation_id": operation.String()}})
		return err
	})
}

// LoadKubernetesInput is an internal recipient-bound lease. A preview may read
// a fresh draft; execution may read only the exact approved running operation.
func (s Service) LoadKubernetesInput(ctx context.Context, enterprise, cluster uuid.UUID, input kubernetesinventory.Mutation, dryRun bool, recipientType, recipientID string) ([]byte, error) {
	if recipientType != "server" && recipientType != "worker" && recipientType != "connector" || recipientID == "" {
		return nil, ErrKubernetesPrivateInput
	}
	if recipientType == "connector" {
		id, e := uuid.Parse(recipientID)
		if e != nil {
			return nil, ErrKubernetesPrivateInput
		}
		current, e := s.Store.Queries.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: enterprise, ID: cluster})
		if e != nil {
			return nil, ErrKubernetesPrivateInput
		}
		expected := current.ConnectorID
		if current.ConnectionMode == "via_bastion" && current.BastionScopeID.Valid {
			scope, e := s.Store.Queries.GetBastionScope(ctx, db.GetBastionScopeParams{EnterpriseID: enterprise, ID: current.BastionScopeID.UUID})
			if e != nil {
				return nil, ErrKubernetesPrivateInput
			}
			expected = scope.ActiveConnectorID
		}
		if current.ConnectionMode == "direct" || !expected.Valid || expected.UUID != id {
			return nil, ErrKubernetesPrivateInput
		}
		recipient, e := s.Store.Queries.GetConnector(ctx, db.GetConnectorParams{EnterpriseID: enterprise, ID: id})
		if e != nil || recipient.Status != "online" {
			return nil, ErrKubernetesPrivateInput
		}
	}
	id, err := uuid.Parse(input.PrivateInputID)
	if err != nil {
		return nil, ErrKubernetesPrivateInput
	}
	row, err := s.Store.Queries.GetKubernetesPrivateInput(ctx, db.GetKubernetesPrivateInputParams{ID: id, EnterpriseID: enterprise})
	if err != nil {
		return nil, ErrKubernetesPrivateInput
	}
	hash, err := kubernetesMutationHash(input)
	if err != nil || row.ClusterID != cluster || row.OperationID.String() != input.OperationRef || subtle.ConstantTimeCompare(row.MutationHash, hash) != 1 {
		return nil, ErrKubernetesPrivateInput
	}
	actor := authorization.Actor{EnterpriseID: enterprise, SubjectID: row.ActorID, SubjectType: row.ActorType, AuthorizationVersion: row.AuthorizationVersion}
	if err = authorizeKubernetesPrivateInput(ctx, s.Store.Queries, actor, cluster, row.NamespaceID, row.NamespaceUid); err != nil {
		return nil, err
	}
	if dryRun {
		if row.Status != "draft" || time.Since(row.CreatedAt.Time) > 15*time.Minute {
			return nil, ErrKubernetesPrivateInput
		}
	} else {
		operation, e := s.Store.Queries.GetKubernetesOperation(ctx, db.GetKubernetesOperationParams{EnterpriseID: enterprise, ID: row.OperationID})
		if e != nil || row.Status != "bound" || operation.Status != "running" && operation.Status != "result_unknown" {
			return nil, ErrKubernetesPrivateInput
		}
		var plan struct {
			Mutation kubernetesinventory.Mutation `json:"mutation"`
		}
		if json.Unmarshal(operation.PrivatePlan, &plan) != nil {
			return nil, ErrKubernetesPrivateInput
		}
		approved, e := kubernetesMutationHash(plan.Mutation)
		if e != nil || subtle.ConstantTimeCompare(approved, hash) != 1 {
			return nil, ErrKubernetesPrivateInput
		}
	}
	var envelope Envelope
	if json.Unmarshal(row.Envelope, &envelope) != nil {
		return nil, ErrKubernetesPrivateInput
	}
	plaintext, err := s.Keyring.DecryptContext(ctx, envelope, kubernetesInputAAD(enterprise, id, row.OperationID))
	if err != nil {
		return nil, ErrKubernetesPrivateInput
	}
	defer clear(plaintext)
	var material struct {
		Values json.RawMessage `json:"values"`
	}
	if json.Unmarshal(plaintext, &material) != nil {
		return nil, ErrKubernetesPrivateInput
	}
	// Persist recipient-only audit metadata before releasing any material.
	err = s.Store.InTx(ctx, func(q *db.Queries) error {
		if e := authorizeKubernetesPrivateInput(ctx, q, actor, cluster, row.NamespaceID, row.NamespaceUid); e != nil {
			return e
		}
		_, e := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}, ActorType: "system", ActorID: recipientID, Action: "kubernetes.private_input.lease", ResourceType: "kubernetes_cluster", ResourceID: cluster.String(), Result: "success", Details: map[string]any{"operation_id": row.OperationID.String(), "dry_run": dryRun, "recipient_type": recipientType, "recipient_id": recipientID}})
		return e
	})
	if err != nil {
		clear(material.Values)
		return nil, err
	}
	return material.Values, nil
}
