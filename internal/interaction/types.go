// Package interaction owns durable questions and versioned plan consent.
// Explicit confirmation activates server-frozen command references; prose alone never grants authority.
package interaction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"regexp"
	"strings"
	"time"
)

type Option struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}
type Question struct {
	ID       string   `json:"id"`
	Header   string   `json:"header"`
	Question string   `json:"question"`
	Options  []Option `json:"options"`
}
type Questions struct {
	Questions []Question `json:"questions"`
}
type PlanStep struct {
	Description string `json:"description"`
	Language    string `json:"language,omitempty"`
	Code        string `json:"code,omitempty"`
	Target      string `json:"target,omitempty"`
}
type Plan struct {
	Title            string                           `json:"title"`
	Description      string                           `json:"description,omitempty"`
	Steps            []PlanStep                       `json:"steps,omitempty"`
	SupersedesID     *uuid.UUID                       `json:"supersedes_id,omitempty"`
	Markdown         string                           `json:"markdown,omitempty"`
	CommandBindings  []remoteexecution.CommandBinding `json:"command_bindings,omitempty"`
	DocumentMarkdown string                           `json:"document_markdown,omitempty"`
	Commands         []PlanCommand                    `json:"commands,omitempty"`
	Execution        *remoteexecution.CommandManifest `json:"execution,omitempty"`
	RequestHash      string                           `json:"request_hash,omitempty"`
}
type PlanCommand struct {
	ID          uuid.UUID `json:"approved_step_id"`
	BlockID     string    `json:"block_id"`
	Target      string    `json:"target"`
	Identity    string    `json:"identity"`
	Interpreter string    `json:"interpreter"`
}
type Answer struct {
	SelectedOption string `json:"selected_option,omitempty"`
	Text           string `json:"text,omitempty"`
	Skipped        bool   `json:"skipped,omitempty"`
}
type Response struct {
	Version     int64             `json:"version"`
	ContentHash string            `json:"content_hash"`
	Action      string            `json:"action"`
	Answers     map[string]Answer `json:"answers,omitempty"`
	Feedback    string            `json:"feedback,omitempty"`
}
type View struct {
	ID                   uuid.UUID       `json:"id"`
	ConversationID       uuid.UUID       `json:"conversation_id"`
	RunID                uuid.UUID       `json:"run_id"`
	FamilyID             uuid.UUID       `json:"family_id"`
	Kind                 string          `json:"kind"`
	Revision             int32           `json:"revision"`
	Status               string          `json:"status"`
	Version              int64           `json:"version"`
	ContentHash          string          `json:"content_hash"`
	Content              json.RawMessage `json:"content"`
	Response             json.RawMessage `json:"response,omitempty"`
	ImplementationRunID  *uuid.UUID      `json:"implementation_run_id,omitempty"`
	ImplementationStatus string          `json:"implementation_status,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func view(r db.ConversationInteraction) View {
	v := View{ID: r.ID, ConversationID: r.ConversationID, RunID: r.RunID, FamilyID: r.FamilyID, Kind: r.Kind, Revision: r.Revision, Status: r.Status, Version: r.Version, ContentHash: r.ContentHash, Content: r.Content, Response: r.Response, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.Kind == "plan" {
		var p Plan
		if json.Unmarshal(r.Content, &p) == nil {
			p.Execution = nil
			p.CommandBindings = nil
			p.RequestHash = ""
			v.Content, _ = json.Marshal(p)
		}
	}
	if r.ImplementationRunID.Valid {
		v.ImplementationRunID = &r.ImplementationRunID.UUID
	}
	return v
}
func invalid() error         { return toolruntime.Error{Kind: "TOOL_INPUT_INVALID"} }
func conflict() error        { return toolruntime.Error{Kind: "RUN_STATE_CONFLICT"} }
func forbidden() error       { return toolruntime.Error{Kind: "TOOL_PERMISSION_DENIED"} }
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func (q Questions) Validate() error {
	if len(q.Questions) < 1 || len(q.Questions) > 3 {
		return invalid()
	}
	seen := map[string]bool{}
	for _, x := range q.Questions {
		if !identifier.MatchString(x.ID) || seen[x.ID] || strings.TrimSpace(x.Header) == "" || len(x.Header) > 128 || strings.TrimSpace(x.Question) == "" || len(x.Question) > 4096 || len(x.Options) > 4 {
			return invalid()
		}
		seen[x.ID] = true
		labels := map[string]bool{}
		for _, o := range x.Options {
			if strings.TrimSpace(o.Label) == "" || len(o.Label) > 256 || len(o.Description) > 2048 || labels[o.Label] {
				return invalid()
			}
			labels[o.Label] = true
		}
	}
	return nil
}
func (p Plan) Validate() error {
	if p.Markdown != "" {
		if strings.TrimSpace(p.Title) == "" || len(p.Title) > 256 || len(p.Markdown) > 256<<10 || len(p.CommandBindings) > 64 {
			return invalid()
		}
		return nil
	}
	if strings.TrimSpace(p.Title) == "" || len(p.Title) > 256 || len(p.Description) > 32768 || len(p.Steps) < 1 || len(p.Steps) > 64 {
		return invalid()
	}
	for _, s := range p.Steps {
		if strings.TrimSpace(s.Description) == "" || len(s.Description) > 8192 || len(s.Code) > 32768 || len(s.Target) > 1024 || len(s.Language) > 64 {
			return invalid()
		}
	}
	return nil
}
func validateAnswers(q Questions, r Response) error {
	if r.Action == "skip" {
		return nil
	}
	if r.Action != "answer" || len(r.Answers) != len(q.Questions) {
		return invalid()
	}
	for _, x := range q.Questions {
		a, ok := r.Answers[x.ID]
		if !ok || len(a.Text) > 16384 {
			return invalid()
		}
		if a.Skipped {
			if a.SelectedOption != "" || a.Text != "" {
				return invalid()
			}
			continue
		}
		if a.SelectedOption == "" && strings.TrimSpace(a.Text) == "" {
			return invalid()
		}
		if a.SelectedOption != "" {
			found := false
			for _, o := range x.Options {
				found = found || o.Label == a.SelectedOption
			}
			if !found {
				return invalid()
			}
		}
	}
	return nil
}
