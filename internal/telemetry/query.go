package telemetry

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
)

const (
	DefaultMaxRows      = 50_000
	HardMaxRows         = 100_000
	DefaultMaxSamples   = 5_000_000
	HardMaxSamples      = 10_000_000
	DefaultMaxSeries    = 100_000
	HardMaxSeries       = 1_000_000
	DefaultMaxScanBytes = int64(256 << 20)
	HardMaxScanBytes    = int64(1 << 30)
	HardMaxResultBytes  = int64(32 << 20)
	DefaultTimeout      = 10 * time.Second
	HardTimeout         = 30 * time.Second
)

type QueryMeta struct {
	SchemaVersion        string   `json:"schema_version"`
	Partial              bool     `json:"partial"`
	PartialReasons       []string `json:"partial_reasons"`
	AppliedResourceCount int      `json:"applied_resource_count"`
	ScannedBytes         int64    `json:"scanned_bytes"`
	ElapsedMS            int64    `json:"elapsed_ms"`
	PlanHash             string   `json:"plan_hash,omitempty"`
	NextCursor           string   `json:"next_cursor,omitempty"`
}

type LogRecord struct {
	Timestamp   time.Time `json:"timestamp"`
	ResourceID  uuid.UUID `json:"resource_id"`
	Severity    string    `json:"severity"`
	Body        string    `json:"body"`
	ServiceName string    `json:"service_name"`
	TraceID     string    `json:"trace_id"`
}
type Overview struct {
	ResourceCount      int   `json:"resource_count"`
	HealthyCollectors  int   `json:"healthy_collectors"`
	DegradedCollectors int   `json:"degraded_collectors"`
	MetricPoints       int64 `json:"metric_points"`
	LogRecords         int64 `json:"log_records"`
	Spans              int64 `json:"spans"`
	WindowSeconds      int   `json:"window_seconds"`
	Partial            bool  `json:"partial"`
}

type OverviewRequest struct {
	Resources            []resourceref.Ref
	SourceKeys           []string
	ScopeSignature       string
	SubjectID            uuid.UUID
	SubjectType          string
	EnterpriseID         uuid.UUID
	ResourceIDs          []uuid.UUID
	AuthorizationVersion int64
	From                 time.Time
	To                   time.Time
	MaxScanBytes         int64
	Timeout              time.Duration
}

type OverviewBackend interface {
	Overview(context.Context, OverviewRequest) (Overview, error)
}

func (service Service) AuthorizedResources(ctx context.Context, actor Actor, requested []uuid.UUID) ([]uuid.UUID, bool, error) {
	if len(requested) > 1000 {
		return nil, false, ErrQueryInvalid
	}
	if service.Store == nil {
		return nil, false, ErrUnavailable
	}
	a := authorization.Actor{EnterpriseID: actor.EnterpriseID, SubjectID: actor.SubjectID, SubjectType: actor.SubjectType, AuthorizationVersion: actor.AuthorizationVersion}
	snapshot, err := (observationscope.Resolver{Queries: service.Store.Queries}).Resolve(ctx, a, observationscope.Selection{Mode: observationscope.Explicit, IDs: requested, Signal: "catalog"})
	if errors.Is(err, observationscope.ErrDenied) {
		return nil, false, ErrDenied
	}
	if errors.Is(err, observationscope.ErrInvalid) {
		return nil, false, ErrQueryInvalid
	}
	return snapshot.IDs(), false, err
}

func (service Service) QueryOverview(ctx context.Context, actor Actor, ids []uuid.UUID, lookback int) (Overview, error) {
	if service.Query == nil || lookback < 300 || lookback > 7*24*60*60 {
		return Overview{}, ErrQueryInvalid
	}
	snapshot, keys, err := service.ResolveQueryScope(ctx, actor, ids, "overview")
	if err != nil {
		return Overview{}, err
	}
	if len(snapshot.Resources) == 0 || len(keys) == 0 {
		if err := service.ValidateResolvedQueryScope(ctx, actor, snapshot); err != nil {
			return Overview{}, err
		}
		return Overview{ResourceCount: len(snapshot.Resources), WindowSeconds: lookback}, nil
	}
	policy, err := service.Store.Queries.EnsureTelemetryRetentionPolicy(ctx, actor.EnterpriseID)
	if err != nil {
		return Overview{}, err
	}
	now := time.Now().UTC()
	request := OverviewRequest{Resources: snapshot.DataRefs(), SourceKeys: keys, ScopeSignature: snapshot.QuerySignature(), SubjectID: actor.SubjectID, SubjectType: actor.SubjectType, EnterpriseID: actor.EnterpriseID, ResourceIDs: snapshot.DataIDs(), AuthorizationVersion: actor.AuthorizationVersion,
		From: now.Add(-time.Duration(lookback) * time.Second), To: now, MaxScanBytes: policy.MaxScanBytes, Timeout: time.Duration(policy.MaxExecutionMs) * time.Millisecond}
	result, err := service.Query.Overview(ctx, request)
	if err != nil {
		return Overview{}, err
	}
	if err := service.ValidateResolvedQueryScope(ctx, actor, snapshot); err != nil {
		return Overview{}, err
	}
	return result, nil
}

var ErrQueryBackend = errors.New("telemetry query backend unavailable")
var ErrQueryProtocol = errors.New("telemetry query protocol version unsupported")
var ErrQuerySourcesEmpty = errors.New("telemetry query has no authorized sources")
