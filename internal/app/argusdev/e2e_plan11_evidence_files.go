package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// A report field named limitations did not prescribe a JSON container type.
// Preserve equivalent nonempty prose/list forms without accepting other types.
type plan11Limitations []string

func (p *plan11Limitations) UnmarshalJSON(raw []byte) error {
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		clean := []string{}
		for _, text := range list {
			if strings.TrimSpace(text) != "" {
				clean = append(clean, text)
			}
		}
		*p = clean
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return errors.New("limitations must be text or a text list")
	}
	if strings.TrimSpace(text) == "" {
		*p = nil
	} else {
		*p = []string{text}
	}
	return nil
}

type plan11QueryFile struct {
	ID     string  `json:"id"`
	Query  string  `json:"query"`
	Hash   string  `json:"hash"`
	Bytes  int     `json:"bytes"`
	FileID *string `json:"file_id"`
	Body   []byte  `json:"-"`
}

// Export immutable originals before interpreting model conclusions. Failed
// parsing/quality checks must not erase the only independently checkable bytes.
func (a *App) plan11LoadQueryFiles(ctx context.Context, env *E2EEnvironment, conversation, run string) ([]plan11QueryFile, error) {
	if uuid.Validate(conversation) != nil || uuid.Validate(run) != nil {
		return nil, errors.New("invalid evidence conversation/run")
	}
	raw, err := a.postgresQuery(ctx, env, `SELECT coalesce(json_agg(json_build_object('id',t.id,'query',t.input->'arguments'->>'query','hash',encode(a.content_hash,'hex'),'bytes',a.byte_size,'file_id',f.id) ORDER BY t.created_at,t.call_sequence),'[]') FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id JOIN artifacts a ON a.id=r.artifact_id LEFT JOIN workspace_files f ON f.enterprise_id=t.enterprise_id AND f.conversation_id=a.conversation_id AND f.deleted_at IS NULL AND f.path='queries/metrics/'||t.id::text||'.json' AND a.result_ref=ANY(f.source_result_refs) WHERE t.run_id='`+run+`' AND t.source='argus' AND t.tool_id='tool.invoke' AND t.status='succeeded' AND t.input->>'category'='metric' AND t.input->>'name'='query'`)
	if err != nil {
		return nil, err
	}
	var files []plan11QueryFile
	if json.Unmarshal([]byte(raw), &files) != nil || len(files) > 64 {
		return nil, errors.New("invalid/unbounded query evidence cohort")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	total := 0
	for i := range files {
		row := &files[i]
		if uuid.Validate(row.ID) != nil || row.Bytes < 1 || row.Bytes > 16<<20 {
			return nil, errors.New("invalid query file facts")
		}
		total += row.Bytes
		if total > 64<<20 {
			return nil, errors.New("query evidence export exceeded its 64MiB cohort limit")
		}
		if row.FileID == nil {
			continue
		}
		if uuid.Validate(*row.FileID) != nil {
			return nil, errors.New("invalid query Workspace reference")
		}
		body, _, e := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+conversation+"/workspace/files/"+*row.FileID+"/content", nil, 200, "")
		if e != nil {
			return nil, e
		}
		if len(body) != row.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != row.Hash {
			return nil, fmt.Errorf("%w: file differs from sealed original", errPlan11ComparisonBoundary)
		}
		if e = writePrivate(filepath.Join(env.Options.Artifacts, "comparison-original-"+row.ID+".json"), body); e != nil {
			return nil, e
		}
		row.Body = body
	}
	return files, nil
}

type plan11ReportFile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Hash      string `json:"hash"`
	MediaType string `json:"media_type"`
	Bytes     int    `json:"bytes"`
	Body      []byte `json:"-"`
}

func (a *App) plan11LoadReportFiles(ctx context.Context, env *E2EEnvironment, conversation, run string) ([]plan11ReportFile, error) {
	if uuid.Validate(conversation) != nil || uuid.Validate(run) != nil {
		return nil, errors.New("invalid report evidence identity")
	}
	raw, err := a.postgresQuery(ctx, env, `SELECT coalesce(json_agg(json_build_object('id',id,'name',name,'media_type',media_type,'bytes',byte_size,'hash',content_hash) ORDER BY created_at,id),'[]') FROM file_deliveries WHERE run_id='`+run+`' AND conversation_id='`+conversation+`' AND deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	var all []plan11ReportFile
	if json.Unmarshal([]byte(raw), &all) != nil || len(all) > 64 {
		return nil, errors.New("invalid/unbounded report cohort")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	files := []plan11ReportFile{}
	for _, row := range all {
		if !strings.HasSuffix(strings.ToLower(row.Name), ".md") && !strings.Contains(strings.ToLower(row.MediaType), "markdown") {
			continue
		}
		if uuid.Validate(row.ID) != nil || row.Bytes < 1 || row.Bytes > 256<<10 {
			return nil, errors.New("invalid or oversized concise report")
		}
		body, _, e := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+conversation+"/deliveries/"+row.ID+"/content", nil, 200, "")
		if e != nil {
			return nil, e
		}
		if len(body) != row.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != row.Hash {
			return nil, fmt.Errorf("%w: report differs from sealed delivery", errPlan11ComparisonBoundary)
		}
		if e = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-report-"+row.ID+".md"), body); e != nil {
			return nil, e
		}
		row.Body = body
		files = append(files, row)
	}
	return files, nil
}
