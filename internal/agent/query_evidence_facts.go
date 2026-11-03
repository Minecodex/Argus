package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type queryEvidenceFile struct {
	CallID                  string `json:"query_call_id"`
	Path                    string `json:"path,omitempty"`
	SHA256                  string `json:"original_sha256"`
	Bytes                   int32  `json:"original_bytes"`
	Available               bool   `json:"available"`
	ReadStatus              string `json:"read_status"`
	InlineOriginalInContext bool   `json:"inline_original_in_context"`
	InlineExposureVerified  bool   `json:"inline_exposure_verified"`
}

// Rebuild from the current Run and current authorization after every inference
// or restart. This is evidence metadata, not a new authority or a final-answer
// gate. Never infer that a successful arbitrary script consumed an entire file.
func (loop Loop) queryEvidenceFacts(ctx context.Context, run db.Run, messages ...modelprovider.Message) (map[string]any, error) {
	scope, err := presentation.Scope(ctx, loop.Store, run.EnterpriseID, run.ActorUserID)
	if err != nil {
		return nil, err
	}
	rows, err := loop.Store.Queries.ListRunNativeQueryEvidence(ctx, db.ListRunNativeQueryEvidenceParams{RunID: run.ID, EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, AuthorizationScope: scope})
	if err != nil {
		return nil, err
	}
	complete := len(rows) <= 64
	if len(rows) > 64 {
		rows = rows[:64]
	}
	items := []queryEvidenceFile{}
	paths := []string{}
	for _, row := range rows {
		if scope == "" || row.AuthorizationScope != scope || len(row.ContentHash) != sha256.Size {
			continue
		}
		expected := toolruntime.NativeQueryFile(row.TargetToolID, row.ToolCallID)
		if expected == "" {
			continue
		}
		item := queryEvidenceFile{CallID: row.ToolCallID.String(), SHA256: hex.EncodeToString(row.ContentHash), Bytes: row.ByteSize, ReadStatus: "not_verified_by_framework"}
		item.Available = row.FilePath == expected && row.FileHash == item.SHA256 && row.FileBytes == int64(row.ByteSize)
		if item.Available {
			item.Path = "/workspace/" + expected
			paths = append(paths, item.Path, expected)
			item.InlineOriginalInContext = currentInlineQueryOriginal(messages, item, row.Projection)
			item.InlineExposureVerified = row.InlineExposureVerified
		}
		items = append(items, item)
	}
	if len(paths) > 0 {
		reads, err := loop.Store.Queries.ListRunQueryFileReads(ctx, db.ListRunQueryFileReadsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID, Paths: paths, AuthorizationScope: scope})
		if err != nil {
			return nil, err
		}
		for _, read := range reads {
			for index := range items {
				if items[index].Available && completeQueryFileRead(read.Input, read.Projection, items[index]) {
					items[index].ReadStatus = "complete_read_verified"
				}
			}
		}
	}
	current, err := presentation.Scope(ctx, loop.Store, run.EnterpriseID, run.ActorUserID)
	if err != nil {
		return nil, err
	}
	if current != scope {
		items = []queryEvidenceFile{}
		complete = false
	}
	return toolruntime.NativeQueryEvidenceFacts(items, complete), nil
}

// Match the currently visible tool message to its trusted persisted projection.
// Old checkpoints/narratives and user/external messages cannot certify presence.
func currentInlineQueryOriginal(messages []modelprovider.Message, file queryEvidenceFile, persistedProjection []byte) bool {
	if !file.Available || file.Bytes <= 0 || file.Bytes > toolruntime.MaxInlineQueryOriginalBytes {
		return false
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		if !toolruntime.QueryJSONEqual([]byte(message.Content), persistedProjection) {
			continue
		}
		var out struct {
			CallID   string `json:"tool_call_id"`
			Hash     string `json:"result_hash"`
			Bytes    int32  `json:"original_bytes"`
			Complete bool   `json:"inline_original_complete"`
			Partial  bool   `json:"partial"`
		}
		if json.Unmarshal([]byte(message.Content), &out) == nil && out.Complete && !out.Partial && out.CallID == file.CallID && out.Hash == file.SHA256 && out.Bytes == file.Bytes {
			return true
		}
	}
	return false
}

func completeQueryFileRead(input, projection []byte, file queryEvidenceFile) bool {
	var in struct{ Path string }
	var out struct {
		Partial bool
		Summary struct {
			Path      string
			Content   string
			ByteSize  int32 `json:"byte_size"`
			StartLine int   `json:"start_line"`
			Partial   bool
		}
	}
	if json.Unmarshal(input, &in) != nil || json.Unmarshal(projection, &out) != nil || out.Partial || out.Summary.Partial || out.Summary.StartLine != 1 || out.Summary.ByteSize != file.Bytes {
		return false
	}
	if strings.TrimPrefix(in.Path, "/workspace/") != strings.TrimPrefix(file.Path, "/workspace/") || strings.TrimPrefix(out.Summary.Path, "/workspace/") != strings.TrimPrefix(in.Path, "/workspace/") {
		return false
	}
	return toolruntime.QueryFileReadMatches(out.Summary.Content, int(out.Summary.ByteSize), out.Summary.StartLine, out.Partial || out.Summary.Partial, file.SHA256, int(file.Bytes))
}
