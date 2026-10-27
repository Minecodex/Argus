package monitoring

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func publicCollectionInput(in *CollectionInput) map[string]any {
	if in == nil {
		return nil
	}
	raw, _ := json.Marshal(in)
	value := map[string]any{}
	_ = json.Unmarshal(raw, &value)
	delete(value, "credential_draft_id")
	if in.Target.ID == uuid.Nil {
		delete(value, "target")
	}
	if in.ApplicationID == uuid.Nil {
		delete(value, "application_id")
	}
	if in.ServiceID == uuid.Nil {
		delete(value, "service_id")
	}
	return value
}

func (s Service) previewTransaction(ctx context.Context, a Actor, m Mutation, credentials []byte, key string) (db.PendingAction, error) {
	if len(credentials) > 0 && (m.Kind != "collection" || m.Collection == nil || m.Collection.CredentialUpdate != "replace") {
		return db.PendingAction{}, ErrCredential
	}
	if m.Collection != nil && m.Collection.CredentialUpdate == "replace" {
		if m.Collection.CollectorID == uuid.Nil {
			return db.PendingAction{}, ErrInvalid
		}
		if monitoringcatalog.ValidateCredentials(m.Collection.Integration, credentials) != nil {
			return db.PendingAction{}, ErrCredential
		}
	}
	mac := hmac.New(sha256.New, s.Actions.Key)
	mac.Write(credentials)
	request := struct {
		Mutation         Mutation
		CredentialDigest string
	}{m, hex.EncodeToString(mac.Sum(nil))}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return db.PendingAction{}, err
	}
	replay, err := s.Actions.Idempotency.Lookup(ctx, s.Store.Queries, "enterprise", a.SubjectType+":"+a.SubjectID.String(), "monitoring."+m.Kind+"."+m.Operation+".preview", key, requestJSON)
	if err != nil {
		return db.PendingAction{}, err
	}
	if replay.Replay {
		var prior db.PendingAction
		err = json.Unmarshal(replay.Body, &prior)
		return prior, err
	}
	ctx, err = s.prepareExternalReads(ctx, a, m)
	if err != nil {
		return db.PendingAction{}, err
	}
	result, err := postgres.ExecuteIdempotent(ctx, s.Store, s.Actions.Idempotency, "enterprise", a.SubjectType+":"+a.SubjectID.String(), "monitoring."+m.Kind+"."+m.Operation+".preview", key, request, 201, func(q *db.Queries) (db.PendingAction, error) {
		// Authorization and draft creation share the action transaction. A failed
		// target/collector check rolls back the encrypted draft as well.
		if m.Collection != nil && len(credentials) > 0 {
			copy := *m.Collection
			m.Collection = &copy
			draftID := uuid.New()
			id, err := s.Credentials.StageMonitoringWithQueries(ctx, q, a.EnterpriseID, a.SubjectID, draftID, credentials)
			if err != nil {
				return db.PendingAction{}, prepareFailure("credential_encryption", err)
			}
			targetHash, err := credentialTargetHash(m.Collection)
			if err != nil {
				return db.PendingAction{}, err
			}
			_, err = q.CreateMonitoringCredentialDraft(ctx, db.CreateMonitoringCredentialDraftParams{ID: draftID, EnterpriseID: a.EnterpriseID, EditorType: a.SubjectType, EditorID: a.SubjectID, CollectorID: m.Collection.CollectorID, SecretID: id, TargetHash: targetHash, ExpiresAt: pgtype.Timestamptz{Time: s.now().Add(20 * time.Minute), Valid: true}})
			if err != nil {
				return db.PendingAction{}, prepareFailure("credential_draft_storage", err)
			}
			m.Collection.CredentialDraftID = draftID
		}
		plan, err := s.prepare(ctx, q, a, m, true)
		if err != nil {
			return db.PendingAction{}, prepareFailure("domain_preparation", err)
		}
		return s.Actions.PrepareWithQueries(ctx, q, resource.ActionSubject{ID: a.SubjectID.String(), Type: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ConfirmationRequired: true}, a.EnterpriseID, pendingInput(plan))
	})
	return result, lookupError(err)
}

func credentialTargetHash(in *CollectionInput) ([]byte, error) {
	// Canonical identity excludes sampling settings. Credentials cannot be reused
	// when editing a different destination or changing the executing Collector.
	identity := in.TargetIdentity
	if len(identity) == 0 {
		identity = json.RawMessage(`{}`)
	}
	raw, err := json.Marshal(struct {
		Integration string
		Target      ResourceRef
		Identity    json.RawMessage
		Collector   uuid.UUID
	}{in.Integration, in.Target, identity, in.CollectorID})
	if err != nil {
		return nil, ErrInvalid
	}
	raw, err = resource.CanonicalJSON(raw)
	if err != nil {
		return nil, ErrInvalid
	}
	hash := sha256.Sum256(raw)
	return hash[:], nil
}
