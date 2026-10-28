// Package modelmetrics owns the private, replayable model-governance projection.
// It never accepts caller-authored PromQL or an externally supplied data scope.
package modelmetrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

const TemplateKey = "argus.model-governance"
const TemplateVersion int64 = 1
const SourceType = "argus_internal_model_governance"
const ProtocolVersion = "argus.model_governance/v1"

var ErrDenied = errors.New("MODEL_GOVERNANCE_DENIED")
var ErrInvalid = errors.New("MODEL_GOVERNANCE_INVALID")
var ErrBudget = errors.New("MODEL_GOVERNANCE_BUDGET")
var ErrUnavailable = errors.New("MODEL_GOVERNANCE_UNAVAILABLE")

type Request struct {
	Actor          authorization.Actor `json:"actor"`
	TemplateHash   string              `json:"template_hash"`
	Month          string              `json:"month"`
	ModelIDs       []uuid.UUID         `json:"model_ids"`
	ModelMode      string              `json:"model_mode"`
	PanelIDs       []string            `json:"panel_ids"`
	CandidatesOnly bool                `json:"candidates_only"`
}
type Model struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}
type Subject struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	DepartmentID uuid.UUID `json:"department_id"`
}
type PanelResult struct {
	ID          string             `json:"id"`
	Result      queryengine.Result `json:"result"`
	Unavailable bool               `json:"unavailable"`
}
type Response struct {
	From                time.Time     `json:"from"`
	To                  time.Time     `json:"to"`
	Timezone            string        `json:"timezone"`
	Models              []Model       `json:"models"`
	Subjects            []Subject     `json:"subjects,omitempty"`
	Panels              []PanelResult `json:"panels,omitempty"`
	Pending             int64         `json:"pending"`
	ProjectedAt         *time.Time    `json:"projected_at"`
	UsageComplete       bool          `json:"usage_complete"`
	CachedUsageComplete bool          `json:"cached_usage_complete"`
	EstimatedInput      int64         `json:"estimated_input"`
	EstimatedOutput     int64         `json:"estimated_output"`
	ScopeHash           string        `json:"scope_hash"`
}
type Backend interface {
	QueryModelGovernance(context.Context, Request) (Response, error)
}
type Panel struct {
	ID, Title, Chart, Expression, Unit string
	Trend                              bool
}

// This registry is consumed by both the immutable Dashboard Spec and the metric
// service. Queries cannot be substituted by a browser, saved draft or AI tool.
func Panels() []Panel {
	return []Panel{
		{"requests", "调用数", "stat", "sum(argus_model_requests)", "", false},
		{"tokens", "实报 Token", "stat", "sum(argus_model_input_tokens) + sum(argus_model_output_tokens)", "", false},
		{"cached", "缓存输入 Token", "stat", "sum(argus_model_cached_tokens)", "", false},
		{"amount", "结算金额", "stat", "sum(argus_model_amount)", "", false},
		{"success", "成功率", "stat", "sum(argus_model_successes) / sum(argus_model_requests)", "percent_ratio", false},
		{"latency", "平均调用延迟", "stat", "sum(argus_model_latency_ms) / sum(argus_model_latency_count)", "ms", false},
		{"errors", "失败调用", "stat", "sum(argus_model_errors)", "", false},
		{"cancelled", "取消调用", "stat", "sum(argus_model_cancelled)", "", false},
		{"compaction", "压缩调用", "stat", "sum(argus_model_compactions)", "", false},
		{"requests_trend", "每日调用数", "timeseries", "sum(argus_model_requests)", "", true},
		{"tokens_trend", "每日实报 Token", "timeseries", "sum(argus_model_input_tokens) + sum(argus_model_output_tokens)", "", true},
		{"amount_trend", "每日结算金额", "timeseries", "sum(argus_model_amount)", "", true},
		{"p95", "每日 P95 延迟", "timeseries", "argus_model_latency_p95_ms", "ms", true},
		{"models", "模型金额排行", "bar", "topk(20, sum by (model_id, model) (argus_model_amount))", "", false},
		{"departments", "部门金额排行", "bar", "topk(20, sum by (department_id, department) (argus_model_amount))", "", false},
		{"users", "用户金额排行", "table", "topk(50, sum by (user_id, user) (argus_model_amount))", "", false},
		{"quota", "所选月份额度占用（当前限额）", "bar_gauge", "argus_model_quota_used / argus_model_quota_limit", "percent_ratio", false},
		{"tool_failures", "工具调用失败", "stat", "argus_model_tool_failures", "", false},
		{"structured_failures", "结构化输出失败", "stat", "argus_model_structured_failures", "", false},
	}
}
func ContractHash() string {
	data, _ := json.Marshal(Panels())
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func DashboardID(enterprise uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(enterprise, []byte("argus-managed-dashboard/"+TemplateKey))
}

type Call struct {
	EnterpriseID uuid.UUID `json:"enterprise_id"`
	ID           uuid.UUID `json:"id"`
	ModelID      uuid.UUID `json:"model_id"`
	DepartmentID uuid.UUID `json:"department_id"`
	UserID       uuid.UUID `json:"user_id"`
	CompletedAt  time.Time `json:"completed_at"`
	Status       string    `json:"status"`
	CallKind     string    `json:"call_kind"`
	ErrorCode    string    `json:"error_code"`
	Input        int64     `json:"input_tokens"`
	Output       int64     `json:"output_tokens"`
	Cached       int64     `json:"cached_tokens"`
	InputSource  string    `json:"input_source"`
	OutputSource string    `json:"output_source"`
	CachedSource string    `json:"cached_source"`
	Amount       string    `json:"amount"`
	LatencyMS    int64     `json:"latency_ms"`
	Dispatched   bool      `json:"dispatched"`
}
type Quota struct {
	EnterpriseID uuid.UUID `json:"enterprise_id"`
	ID           uuid.UUID `json:"id"`
	ModelID      uuid.UUID `json:"model_id"`
	SubjectType  string    `json:"subject_type"`
	SubjectID    uuid.UUID `json:"subject_id"`
	Amount       string    `json:"amount"`
	EffectiveAt  time.Time `json:"effective_at"`
	Deleted      bool      `json:"deleted"`
}
type Reservation struct {
	EnterpriseID uuid.UUID `json:"enterprise_id"`
	ID           uuid.UUID `json:"id"`
	ModelID      uuid.UUID `json:"model_id"`
	DepartmentID uuid.UUID `json:"department_id"`
	UserID       uuid.UUID `json:"user_id"`
	Month        string    `json:"month"`
	Reserved     string    `json:"reserved_amount"`
	Settled      string    `json:"settled_amount"`
	Status       string    `json:"status"`
}
