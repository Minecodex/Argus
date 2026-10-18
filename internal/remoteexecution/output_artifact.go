package remoteexecution

import (
	"time"

	"github.com/google/uuid"
)

const (
	ResultsDirectory    = "results"
	ResultsMetadataPath = ResultsDirectory + "/meta.json"
)

// OutputArtifact contains service-observed facts about sealed remote bytes.
// It is an internal import contract, never a model-authored authority claim.
type OutputArtifact struct {
	ID              uuid.UUID `json:"artifact_id"`
	RequestID       uuid.UUID `json:"request_id"`
	RunID           uuid.UUID `json:"run_id"`
	StepID          uuid.UUID `json:"step_id"`
	SessionID       uuid.UUID `json:"session_id"`
	Target          Target    `json:"resource"`
	RequestTitle    string    `json:"request_title"`
	StepDescription string    `json:"step_description"`
	SourcePath      string    `json:"source_path"`
	ByteSize        int64     `json:"byte_size"`
	ContentHash     string    `json:"content_hash"`
	CollectedAt     time.Time `json:"collected_at"`
}

func (a OutputArtifact) SourceRef() string { return "remote:" + a.RequestID.String() }
