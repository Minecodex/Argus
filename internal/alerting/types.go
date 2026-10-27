// Package alerting owns alert policies, evaluation, noise decisions and delivery.
package alerting

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardaccess"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
)

var (
	ErrInvalid     = errors.New("ALERT_INVALID")
	ErrDenied      = errors.New("ALERT_DENIED")
	ErrNotFound    = errors.New("ALERT_NOT_FOUND")
	ErrConflict    = errors.New("ALERT_VERSION_CONFLICT")
	ErrEvaluator   = errors.New("ALERT_EVALUATOR_UNAVAILABLE")
	ErrUnavailable = errors.New("ALERT_UNAVAILABLE")
)

type Actor = dashboardaccess.Actor
type Service struct {
	Store     *postgres.Store
	Actions   resource.PendingActionService
	Telemetry telemetry.Service
	Secrets   secret.Service
}

type Matcher struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}
type ResourceSelector struct {
	Mode      string      `json:"mode"`
	Types     []string    `json:"types"`
	IDs       []uuid.UUID `json:"ids"`
	Matchers  []Matcher   `json:"matchers"`
	SourceIDs []uuid.UUID `json:"source_ids"`
}
type Rule struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Expression         string    `json:"expression"`
	QueryMode          string    `json:"query_mode"`
	PresenceExpression string    `json:"presence_expression,omitempty"`
	Operator           string    `json:"operator"`
	Threshold          float64   `json:"threshold"`
	RecoveryThreshold  float64   `json:"recovery_threshold"`
	Severity           string    `json:"severity"`
	Unit               string    `json:"unit"`
}
type PolicySpec struct {
	Description     string            `json:"description"`
	Enabled         bool              `json:"enabled"`
	Resources       ResourceSelector  `json:"resources"`
	Rules           []Rule            `json:"rules"`
	Mode            string            `json:"mode"`
	MatchBy         []string          `json:"match_by"`
	IntervalSeconds int64             `json:"interval_seconds"`
	ForSeconds      int64             `json:"for_seconds"`
	RecoverySeconds int64             `json:"recovery_seconds"`
	NoDataBehavior  string            `json:"no_data_behavior"`
	ErrorBehavior   string            `json:"error_behavior"`
	ChannelIDs      []uuid.UUID       `json:"channel_ids"`
	RouteIDs        []uuid.UUID       `json:"route_ids"`
	Labels          map[string]string `json:"labels"`
}
type SettingsSpec struct {
	EvaluatorID uuid.UUID `json:"evaluator_id"`
}
type SilenceSpec struct {
	Enabled     bool        `json:"enabled"`
	ResourceIDs []uuid.UUID `json:"resource_ids"`
	PolicyIDs   []uuid.UUID `json:"policy_ids"`
	Metric      string      `json:"metric"`
	Matchers    []Matcher   `json:"matchers"`
	StartsAt    time.Time   `json:"starts_at"`
	EndsAt      time.Time   `json:"ends_at"`
	Reason      string      `json:"reason"`
}
type InhibitionSpec struct {
	ResourceIDs []uuid.UUID `json:"resource_ids"`
	Enabled     bool        `json:"enabled"`
	Source      []Matcher   `json:"source"`
	Target      []Matcher   `json:"target"`
	Equal       []string    `json:"equal"`
	Reason      string      `json:"reason"`
}
type ChannelSpec struct {
	Enabled               bool      `json:"enabled"`
	DefaultForNewPolicies bool      `json:"default_for_new_policies"`
	Provider              string    `json:"provider"`
	CredentialID          uuid.UUID `json:"credential_id"`
	DisplayAddress        string    `json:"display_address"`
	MentionIDs            []string  `json:"mention_ids"`
	MentionAll            bool      `json:"mention_all"`
	Template              string    `json:"template"`
}
type RouteSpec struct {
	Enabled    bool        `json:"enabled"`
	Matchers   []Matcher   `json:"matchers"`
	ChannelIDs []uuid.UUID `json:"channel_ids"`
	Continue   bool        `json:"continue"`
	Priority   int         `json:"priority"`
}
type GroupingSpec struct {
	GroupBy         []string `json:"group_by"`
	WaitSeconds     int64    `json:"wait_seconds"`
	IntervalSeconds int64    `json:"interval_seconds"`
	RepeatSeconds   int64    `json:"repeat_seconds"`
	Timing          []string `json:"timing"`
}
type TemplateSpec struct {
	Content string `json:"content"`
}

type Configuration struct {
	ManagedBy        string          `json:"managed_by,omitempty"`
	MatchedInstances int64           `json:"matched_instances,omitempty"`
	ID               uuid.UUID       `json:"id"`
	Kind             string          `json:"kind"`
	Name             string          `json:"name"`
	Spec             json.RawMessage `json:"spec"`
	Version          int64           `json:"version"`
	Generation       int64           `json:"generation"`
	Health           string          `json:"health"`
	ErrorCode        string          `json:"error_code,omitempty"`
	LastEvaluatedAt  *time.Time      `json:"last_evaluated_at,omitempty"`
	LastSuccessfulAt *time.Time      `json:"last_successful_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}
type Mutation struct {
	Kind            string          `json:"kind"`
	Operation       string          `json:"operation"`
	ID              uuid.UUID       `json:"id"`
	Name            string          `json:"name"`
	Spec            json.RawMessage `json:"spec"`
	ExpectedVersion int64           `json:"expected_version"`
}
type ActionPlan struct {
	References       map[string]int64 `json:"references"`
	Mutation         Mutation         `json:"mutation"`
	Actor            Actor            `json:"actor"`
	ResourceIDs      []uuid.UUID      `json:"resource_ids"`
	EvaluatorID      uuid.UUID        `json:"evaluator_id"`
	EvaluatorVersion int64            `json:"evaluator_version"`
}
type MemberValue struct {
	RuleID            uuid.UUID `json:"rule_id"`
	Name              string    `json:"name"`
	Expression        string    `json:"expression"`
	Value             float64   `json:"value"`
	Threshold         float64   `json:"threshold"`
	RecoveryThreshold float64   `json:"recovery_threshold"`
	Severity          string    `json:"severity"`
	Firing            bool      `json:"firing"`
	Recovering        bool      `json:"recovering"`
	Unit              string    `json:"unit"`
}

type Observation struct {
	Members     []MemberValue     `json:"members,omitempty"`
	Key         string            `json:"key"`
	Labels      map[string]string `json:"labels"`
	Value       float64           `json:"value"`
	RuleID      uuid.UUID         `json:"rule_id"`
	ResourceIDs []uuid.UUID       `json:"resource_ids"`
	SourceIDs   []uuid.UUID       `json:"source_ids"`
	Firing      bool              `json:"firing"`
	Recovering  bool              `json:"recovering"`
	Severity    string            `json:"severity"`
	Rule        Rule              `json:"rule"`
}
type InstanceState struct {
	Members              []MemberValue     `json:"members,omitempty"`
	EvaluatorID          uuid.UUID         `json:"evaluator_id"`
	AuthorizationVersion int64             `json:"authorization_version"`
	SourceSnapshot       json.RawMessage   `json:"source_snapshot,omitempty"`
	LatestSourceSnapshot json.RawMessage   `json:"latest_source_snapshot,omitempty"`
	Name                 string            `json:"name"`
	PolicyName           string            `json:"policy_name"`
	RuleID               uuid.UUID         `json:"rule_id"`
	Metric               string            `json:"metric"`
	Labels               map[string]string `json:"labels"`
	Value                *float64          `json:"value"`
	Threshold            float64           `json:"threshold"`
	RecoveryThreshold    float64           `json:"recovery_threshold"`
	Unit                 string            `json:"unit"`
	PendingSince         *time.Time        `json:"pending_since,omitempty"`
	RecoveringSince      *time.Time        `json:"recovering_since,omitempty"`
	LastSlot             time.Time         `json:"last_slot"`
	// LastSuccessfulAt dates the successful evaluation that produced Value and
	// Members. Failed evaluations retain it; it is not a telemetry sample time.
	LastSuccessfulAt   *time.Time                         `json:"last_successful_at,omitempty"`
	PolicySnapshot     json.RawMessage                    `json:"policy_snapshot"`
	NotificationState  string                             `json:"notification_state"`
	NoiseReasons       []string                           `json:"noise_reasons"`
	LastNotificationAt *time.Time                         `json:"last_notification_at,omitempty"`
	ClosureReason      string                             `json:"closure_reason,omitempty"`
	Kind               string                             `json:"kind"`
	DeliveryGap        *telemetry.CollectorDeliveryReport `json:"delivery_gap,omitempty"`
}
type Instance struct {
	HandlingVersion int64         `json:"handling_version"`
	ID              uuid.UUID     `json:"id"`
	PolicyID        uuid.UUID     `json:"policy_id"`
	PolicyVersion   int64         `json:"policy_version"`
	ResourceIDs     []uuid.UUID   `json:"resource_ids"`
	SourceIDs       []uuid.UUID   `json:"source_ids"`
	State           InstanceState `json:"state"`
	Lifecycle       string        `json:"lifecycle"`
	Health          string        `json:"health"`
	Severity        string        `json:"severity"`
	Acknowledged    bool          `json:"acknowledged"`
	OwnerID         *uuid.UUID    `json:"owner_id"`
	StartedAt       *time.Time    `json:"started_at"`
	ResolvedAt      *time.Time    `json:"resolved_at"`
	LastEvaluatedAt time.Time     `json:"last_evaluated_at,omitempty"`
	Version         int64         `json:"version"`
}
type Event struct {
	ID         uuid.UUID       `json:"id"`
	InstanceID uuid.UUID       `json:"instance_id"`
	EventType  string          `json:"event_type"`
	Payload    json.RawMessage `json:"payload"`
	ActorID    *uuid.UUID      `json:"actor_id"`
	Origin     string          `json:"origin"`
	CreatedAt  time.Time       `json:"created_at"`
}
type HandleInput struct {
	Operation               string     `json:"operation"`
	OwnerID                 *uuid.UUID `json:"owner_id"`
	Comment                 string     `json:"comment"`
	ExpectedHandlingVersion int64      `json:"expected_handling_version"`
}
type ListFilter struct {
	ChannelID         uuid.UUID  `json:"channel_id"`
	Query             string     `json:"query"`
	Severity          string     `json:"severity"`
	Lifecycle         string     `json:"lifecycle"`
	NotificationState string     `json:"notification_state"`
	PolicyID          uuid.UUID  `json:"policy_id"`
	RuleID            uuid.UUID  `json:"rule_id"`
	ResourceID        uuid.UUID  `json:"resource_id"`
	Metric            string     `json:"metric"`
	From              *time.Time `json:"from"`
	To                *time.Time `json:"to"`
	Acknowledged      *bool      `json:"acknowledged"`
	Limit             int        `json:"limit"`
	Cursor            string     `json:"cursor"`
}
type Page[T any] struct {
	ResourceIDs []uuid.UUID        `json:"resource_ids"`
	Evaluations []EvaluationStatus `json:"evaluations,omitempty"`
	Items       []T                `json:"items"`
	Total       int                `json:"total"`
	NextCursor  *string            `json:"next_cursor"`
	Complete    bool               `json:"complete"`
}

type EvaluationStatus struct {
	PolicyID         uuid.UUID  `json:"policy_id"`
	Name             string     `json:"name"`
	Health           string     `json:"health"`
	ErrorCode        string     `json:"error_code,omitempty"`
	LastEvaluatedAt  *time.Time `json:"last_evaluated_at,omitempty"`
	LastSuccessfulAt *time.Time `json:"last_successful_at,omitempty"`
}
type Summary struct {
	CurrentTotal     int        `json:"current_total"`
	Active           int        `json:"active"`
	Unacknowledged   int        `json:"unacknowledged"`
	Policies         int        `json:"policies"`
	FailedDeliveries int        `json:"failed_deliveries"`
	LastEvaluatedAt  *time.Time `json:"last_evaluated_at,omitempty"`
}
type Message struct {
	BatchID         uuid.UUID         `json:"batch_id,omitempty"`
	Initiator       *Actor            `json:"initiator,omitempty"`
	RoutingSnapshot map[string]int64  `json:"routing_snapshot,omitempty"`
	Subject         string            `json:"subject"`
	Text            string            `json:"text"`
	EventType       string            `json:"event_type"`
	InstanceIDs     []uuid.UUID       `json:"instance_ids"`
	PolicyIDs       []uuid.UUID       `json:"policy_ids"`
	Labels          map[string]string `json:"labels"`
	Test            bool              `json:"test"`
}
type Delivery struct {
	BatchID       *uuid.UUID  `json:"batch_id,omitempty"`
	ResendOfID    *uuid.UUID  `json:"resend_of_id,omitempty"`
	ID            uuid.UUID   `json:"id"`
	InstanceID    *uuid.UUID  `json:"instance_id"`
	ChannelID     uuid.UUID   `json:"channel_id"`
	ResourceIDs   []uuid.UUID `json:"resource_ids"`
	Message       Message     `json:"message"`
	Status        string      `json:"status"`
	Attempt       int32       `json:"attempt"`
	LastError     string      `json:"last_error"`
	LastAttemptAt *time.Time  `json:"last_attempt_at"`
	CreatedAt     time.Time   `json:"created_at"`
	Version       int64       `json:"version"`
}
