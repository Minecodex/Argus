// Package remoteexecution owns immutable, human-approved remote batches and
// serial plans. It never grants authority to model-generated approval fields.
package remoteexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	SchemaVersion       = "argus.remote_execution/v1"
	InputDirectory      = "remote/input"
	DefaultIdleTimeout  = 30 * time.Minute
	DefaultBatchTimeout = 5 * time.Minute
	MaxBatchTimeout     = 30 * time.Minute
	MaxSteps            = 64
	MaxScriptBytes      = 256 << 10
	MaxOutputBytes      = 8 << 20
	MaxTransferBytes    = 64 << 20
	MaxFiles            = 256
	ChunkBytes          = 64 << 10
)

type Target struct {
	Name            string    `json:"name,omitempty"`
	ResourceVersion int64     `json:"resource_version,omitempty"`
	ResourceType    string    `json:"resource_type"`
	ResourceID      uuid.UUID `json:"resource_id"`
	Namespace       string    `json:"namespace,omitempty"`
	Pod             string    `json:"pod,omitempty"`
	PodUID          string    `json:"pod_uid,omitempty"`
	Container       string    `json:"container,omitempty"`
}
type Step struct {
	ID                 uuid.UUID    `json:"id"`
	Description        string       `json:"description"`
	Target             Target       `json:"target"`
	Interpreter        string       `json:"interpreter"`
	Script             string       `json:"script"`
	TimeoutSeconds     int          `json:"timeout_seconds"`
	SessionID          uuid.UUID    `json:"session_id"`
	Generation         uuid.UUID    `json:"session_generation"`
	ContextRevision    int64        `json:"context_revision"`
	KnownDirectory     string       `json:"known_directory,omitempty"`
	InterpreterVersion string       `json:"interpreter_version,omitempty"`
	PreviousRequestID  *uuid.UUID   `json:"previous_request_id,omitempty"`
	PreviousSequence   int          `json:"previous_sequence,omitempty"`
	Identity           IdentityView `json:"identity"`
}
type IdentityView struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
	SID      string    `json:"sid,omitempty"`
	Version  int64     `json:"version"`
	Platform string    `json:"platform"`
}
type File struct {
	ID             uuid.UUID `json:"id"`
	ContentPreview string    `json:"content_preview,omitempty"`
	WorkspaceID    uuid.UUID `json:"workspace_id,omitempty"`
	Path           string    `json:"path"`
	Size           int64     `json:"byte_size"`
	Hash           string    `json:"content_hash"`
	ObjectKey      string    `json:"-"`
	SourceRef      string    `json:"source_ref,omitempty"`
}
type Spec struct {
	ConversationID uuid.UUID  `json:"conversation_id"`
	RunID          uuid.UUID  `json:"run_id"`
	SchemaVersion  string     `json:"schema_version"`
	ID             uuid.UUID  `json:"id"`
	Kind           string     `json:"kind"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Steps          []Step     `json:"steps"`
	Inputs         []File     `json:"inputs"`
	SupersedesID   *uuid.UUID `json:"supersedes_id,omitempty"`
	RecoveryOfID   *uuid.UUID `json:"recovery_of_id,omitempty"`
}
type Input struct {
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Steps        []Step     `json:"steps"`
	SupersedesID *uuid.UUID `json:"supersedes_id,omitempty"`
	RecoveryOfID *uuid.UUID `json:"recovery_of_id,omitempty"`
}
type StepResult struct {
	Terminated       bool      `json:"terminated"`
	OutputsCollected bool      `json:"outputs_collected"`
	Sequence         int       `json:"sequence"`
	Status           string    `json:"status"`
	ExitCode         int       `json:"exit_code"`
	Stdout           string    `json:"stdout"`
	Stderr           string    `json:"stderr"`
	Truncated        bool      `json:"truncated"`
	Directory        string    `json:"directory"`
	SessionLost      bool      `json:"session_lost"`
	ErrorCode        string    `json:"error_code,omitempty"`
	OutputFiles      []File    `json:"output_files"`
	ObservedAt       time.Time `json:"observed_at"`
}
type View struct {
	ConfirmationRequired bool         `json:"confirmation_required"`
	StopStatus           string       `json:"stop_status"`
	ID                   uuid.UUID    `json:"id"`
	ActionRef            string       `json:"action_ref"`
	ContentHash          string       `json:"content_hash"`
	Status               string       `json:"status"`
	StopRequested        bool         `json:"stop_requested"`
	Revision             int          `json:"revision"`
	Spec                 Spec         `json:"spec"`
	Results              []StepResult `json:"results"`
	ErrorCode            string       `json:"error_code,omitempty"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

func (i *Input) Validate(kind string) error {
	if (kind != "connect" && kind != "plan") || strings.TrimSpace(i.Title) == "" || len(i.Title) > 256 || len(i.Description) > 16384 || len(i.Steps) == 0 || len(i.Steps) > MaxSteps || kind == "connect" && len(i.Steps) != 1 {
		return Error("REMOTE_INPUT_INVALID")
	}
	for n := range i.Steps {
		s := &i.Steps[n]
		if s.Target.ResourceID == uuid.Nil || (s.Target.ResourceType != "host" && s.Target.ResourceType != "kubernetes_cluster") || strings.TrimSpace(s.Script) == "" || len(s.Script) > MaxScriptBytes || strings.ContainsRune(s.Script, 0) {
			return Error("REMOTE_INPUT_INVALID")
		}
		if s.Target.ResourceType == "host" && (s.Target.Namespace != "" || s.Target.Pod != "" || s.Target.PodUID != "" || s.Target.Container != "") {
			return Error("REMOTE_INPUT_INVALID")
		}
		if s.Target.Namespace != "" && len(validation.IsDNS1123Label(s.Target.Namespace)) > 0 {
			return Error("REMOTE_INPUT_INVALID")
		}
		if s.Target.Pod != "" && len(validation.IsDNS1123Subdomain(s.Target.Pod)) > 0 {
			return Error("REMOTE_INPUT_INVALID")
		}
		if s.Target.Container != "" && len(validation.IsDNS1123Label(s.Target.Container)) > 0 {
			return Error("REMOTE_INPUT_INVALID")
		}
		if s.TimeoutSeconds == 0 {
			s.TimeoutSeconds = int(DefaultBatchTimeout.Seconds())
		}
		if s.TimeoutSeconds < 1 || s.TimeoutSeconds > int(MaxBatchTimeout.Seconds()) {
			return Error("REMOTE_INPUT_INVALID")
		}
		switch s.Interpreter {
		case "bash", "powershell":
			if s.Target.ResourceType != "host" {
				return Error("REMOTE_INPUT_INVALID")
			}
		case "kubectl":
			if s.Target.ResourceType != "kubernetes_cluster" || s.Target.Pod != "" || s.Target.PodUID != "" || s.Target.Container != "" {
				return Error("REMOTE_INPUT_INVALID")
			}
		case "pod_bash":
			if s.Target.ResourceType != "kubernetes_cluster" || s.Target.Namespace == "" || s.Target.Pod == "" || s.Target.PodUID == "" || s.Target.Container == "" {
				return Error("REMOTE_INPUT_INVALID")
			}
		default:
			return Error("REMOTE_INPUT_INVALID")
		}
	}
	return nil
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type Error string

func (e Error) Error() string { return string(e) }
func (e Error) Code() string  { return string(e) }
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", Error("REMOTE_INPUT_INVALID"), fmt.Sprintf(format, args...))
}
