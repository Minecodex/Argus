// Package monitoring owns applications, service identity and collection intent.
// Physical collection and telemetry storage remain in their existing domains.
package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var (
	ErrInvalid        = errors.New("MONITORING_INVALID")
	ErrNotFound       = errors.New("MONITORING_NOT_FOUND")
	ErrConflict       = errors.New("MONITORING_VERSION_CONFLICT")
	ErrDenied         = authorization.ErrAccessDenied
	ErrUnavailable    = errors.New("MONITORING_UNAVAILABLE")
	ErrUnsupported    = errors.New("MONITORING_UNSUPPORTED_INTEGRATION")
	ErrDependency     = errors.New("MONITORING_DEPENDENCIES_EXIST")
	ErrServiceLimit   = errors.New("MONITORING_SERVICE_LIMIT")
	ErrCredential     = errors.New("MONITORING_CREDENTIAL_INVALID")
	ErrIngestRejected = errors.New("APPLICATION_INGEST_REJECTED")
)

type Actor = authorization.Actor

type Service struct {
	TrustBundles IngestTrustProvider
	Store        *postgres.Store
	Actions      resource.PendingActionService
	Credentials  secret.Service
	CursorKey    []byte
	Now          func() time.Time
	Runtime      CollectionRuntime
	Workloads    LogWorkloadResolver
}

type LogWorkloadResolver interface {
	ResolveLogWorkload(context.Context, Actor, uuid.UUID, logcollection.Workload) (logcollection.Workload, error)
}

// CollectionRuntime reuses the existing Collector management transport. The
// intent transaction and command enqueue share the same PostgreSQL transaction.
type CollectionRuntime interface {
	Plan(context.Context, *db.Queries, uuid.UUID, uuid.UUID) (json.RawMessage, error)
	Commit(context.Context, *db.Queries, db.PendingAction, json.RawMessage) (resource.ActionCommitResult, error)
}

type ResourceRef struct {
	Kind string    `json:"kind"`
	ID   uuid.UUID `json:"id"`
}

type Application struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"application_key"`
	Name        string    `json:"name"`
	Environment string    `json:"environment"`
	Description string    `json:"description"`
	OwnerName   string    `json:"owner_name"`
	Status      string    `json:"status"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ApplicationInput struct {
	Key         string `json:"application_key,omitempty"`
	Name        string `json:"name"`
	Environment string `json:"environment,omitempty"`
	Description string `json:"description"`
	OwnerName   string `json:"owner_name"`
}

type ApplicationService struct {
	ID            uuid.UUID  `json:"id"`
	ApplicationID uuid.UUID  `json:"application_id"`
	Namespace     string     `json:"service_namespace"`
	Name          string     `json:"service_name"`
	Runtime       string     `json:"runtime"`
	Status        string     `json:"status"`
	Version       int64      `json:"version"`
	FirstSeenAt   *time.Time `json:"first_seen_at,omitempty"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
}

type ApplicationServiceInput struct {
	ApplicationID uuid.UUID `json:"application_id"`
	Namespace     string    `json:"service_namespace"`
	Name          string    `json:"service_name"`
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type ListInput struct {
	Search         string      `json:"search,omitempty"`
	Cursor         string      `json:"cursor,omitempty"`
	Limit          int         `json:"limit,omitempty"`
	Status         string      `json:"status,omitempty"`
	Integration    string      `json:"integration,omitempty"`
	DesiredEnabled *bool       `json:"desired_enabled,omitempty"`
	ApplyStatus    string      `json:"apply_status,omitempty"`
	Health         string      `json:"health,omitempty"`
	Freshness      string      `json:"freshness,omitempty"`
	ApplicationIDs []uuid.UUID `json:"application_ids,omitempty"`
}

type BindingInput struct {
	ApplicationID uuid.UUID   `json:"application_id"`
	ServiceID     uuid.UUID   `json:"service_id,omitempty"`
	Target        ResourceRef `json:"target"`
}

type CollectionInput struct {
	Name             string          `json:"name"`
	Integration      string          `json:"integration"`
	Target           ResourceRef     `json:"target"`
	TargetName       string          `json:"target_name,omitempty"`
	TargetIdentity   json.RawMessage `json:"target_identity,omitempty"`
	CollectorID      uuid.UUID       `json:"collector_id,omitempty"`
	ApplicationID    uuid.UUID       `json:"application_id,omitempty"`
	ServiceID        uuid.UUID       `json:"service_id,omitempty"`
	Bindings         []BindingInput  `json:"bindings,omitempty"`
	Config           json.RawMessage `json:"config"`
	Enabled          bool            `json:"enabled"`
	CredentialUpdate string          `json:"credential_update"`
	// DraftID refers only to a subject-bound encrypted draft. The public
	// configuration and immutable action never contain authentication values.
	CredentialDraftID uuid.UUID `json:"credential_draft_id,omitempty"`
}

type Mutation struct {
	Service         *ApplicationServiceInput `json:"service,omitempty"`
	Kind            string                   `json:"kind"`
	Operation       string                   `json:"operation"`
	ID              uuid.UUID                `json:"id,omitempty"`
	ExpectedVersion int64                    `json:"expected_version,omitempty"`
	Application     *ApplicationInput        `json:"application,omitempty"`
	Binding         *BindingInput            `json:"binding,omitempty"`
	Collection      *CollectionInput         `json:"collection,omitempty"`
	Endpoint        *IngestEndpointInput     `json:"endpoint,omitempty"`
	IngestKey       *IngestKeyInput          `json:"ingest_key,omitempty"`
}

type actionPlan struct {
	SchemaVersion string            `json:"schema_version"`
	Mutation      Mutation          `json:"mutation"`
	Actor         Actor             `json:"actor"`
	References    map[string]int64  `json:"references"`
	RuntimePlans  []json.RawMessage `json:"runtime_plans,omitempty"`
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
