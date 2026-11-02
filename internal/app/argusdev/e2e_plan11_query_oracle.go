package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type plan11Conclusion struct {
	ResourceID   string                 `json:"resource_id"`
	Metric       string                 `json:"metric"`
	PointCount   *int                   `json:"point_count"`
	FirstPointAt string                 `json:"first_point_at"`
	LastPointAt  string                 `json:"last_point_at"`
	Limitations  plan11Limitations      `json:"limitations"`
	Unit         string                 `json:"unit"`
	ValueMin     *float64               `json:"value_min"`
	ValueMax     *float64               `json:"value_max"`
	DisplayUnit  string                 `json:"display_unit"`
	Transform    string                 `json:"display_transform"`
	DisplayMin   *float64               `json:"display_min"`
	DisplayMax   *float64               `json:"display_max"`
	Dashboards   []plan11DashboardClaim `json:"dashboard_evidence,omitempty"`
}
type plan11PointEvidence struct {
	Count int       `json:"count"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

// This independently checks the downloaded bytes against the sealed artifact
// hash. A mutable Workspace copy cannot redefine the expected query facts.
func (a *App) plan11VerifyQueryConclusion(ctx context.Context, env *E2EEnvironment, conversation, run, host, callsJSON string, conclusion plan11Conclusion) (map[string]any, error) {
	if uuid.Validate(run) != nil || uuid.Validate(host) != nil {
		return nil, errors.New("invalid query oracle identity")
	}
	catalogJSON, err := a.postgresQuery(ctx, env, `SELECT coalesce(json_agg(DISTINCT jsonb_build_object('name',metric->>'name','unit',coalesce(metric->>'unit',''))),'[]') FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id
CROSS JOIN LATERAL jsonb_array_elements(coalesce(r.projection->'summary'->'metrics','[]'::jsonb)) metric
WHERE t.run_id='`+run+`' AND t.tool_id='tool.invoke' AND t.status='succeeded' AND t.input->>'category'='observe' AND t.input->>'name'='catalog.query'`)
	if err != nil {
		return nil, err
	}
	var names []struct{ Name, Unit string }
	if json.Unmarshal([]byte(catalogJSON), &names) != nil {
		return nil, errors.New("metric catalog evidence unavailable")
	}
	found := false
	unit := ""
	for _, name := range names {
		if name.Name == conclusion.Metric {
			if found && unit != name.Unit {
				return nil, errors.New("reported metric has ambiguous source units")
			}
			found, unit = true, name.Unit
		}
	}
	if !found {
		return nil, errors.New("reported metric did not come from the actual catalog")
	}
	raw, err := a.postgresQuery(ctx, env, `SELECT coalesce(json_agg(json_build_object('id',t.id,'query',t.input->'arguments'->>'query','hash',encode(a.content_hash,'hex'),'bytes',a.byte_size,'file_id',f.id) ORDER BY t.created_at,t.call_sequence),'[]')
FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id JOIN artifacts a ON a.id=r.artifact_id
LEFT JOIN workspace_files f ON f.enterprise_id=t.enterprise_id AND f.conversation_id=a.conversation_id AND f.deleted_at IS NULL
 AND f.path='queries/metrics/'||t.id::text||'.json' AND a.result_ref=ANY(f.source_result_refs)
WHERE t.run_id='`+run+`' AND t.source='argus' AND t.tool_id='tool.invoke' AND t.status='succeeded'
 AND t.input->>'category'='metric' AND t.input->>'name'='query'`)
	if err != nil {
		return nil, err
	}
	var facts []struct {
		ID, Query, Hash string
		Bytes           int
		FileID          *string `json:"file_id"`
	}
	if json.Unmarshal([]byte(raw), &facts) != nil || len(facts) == 0 {
		return nil, errors.New("sealed metric evidence unavailable")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	var mismatches []error
	for _, fact := range facts {
		if !strings.Contains(fact.Query, conclusion.Metric) {
			continue
		}
		if fact.FileID == nil || uuid.Validate(*fact.FileID) != nil {
			return nil, errors.New("metric file was not delivered")
		}
		body, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+conversation+"/workspace/files/"+*fact.FileID+"/content", nil, 200, "")
		if err != nil {
			return nil, err
		}
		if len(body) != fact.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != fact.Hash {
			return nil, errors.New("Workspace metric copy differs from immutable query result")
		}
		id, err := uuid.Parse(fact.ID)
		if err != nil {
			return nil, err
		}
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-original-"+id.String()+".json"), body); err != nil {
			return nil, err
		}
		points, err := plan11OriginalPointEvidence(body, host)
		if err != nil {
			return nil, err
		}
		if err = plan11MatchPointConclusion(conclusion, points); err != nil {
			mismatches = append(mismatches, err)
			continue
		}
		values, err := plan11OriginalValueEvidence(body, conclusion.Metric)
		if err != nil {
			mismatches = append(mismatches, err)
			continue
		}
		if err = plan11MatchNumericConclusion(conclusion, unit, values); err != nil {
			mismatches = append(mismatches, err)
			continue
		}
		if !plan11ReadActualQuery(callsJSON, "/workspace/"+toolruntime.NativeQueryFile("telemetry.promql.query", id)) {
			mismatches = append(mismatches, errors.New("actual metric file has no successful read/bash evidence"))
			continue
		}
		if !plan11ComputedStatistics(callsJSON, "/workspace/"+toolruntime.NativeQueryFile("telemetry.promql.query", id)) {
			mismatches = append(mismatches, errors.New("metric statistics have no successful offline script evidence"))
			continue
		}
		return map[string]any{"query_call_id": id, "content_hash": fact.Hash, "returned_points": points, "returned_values": values, "unit": unit, "original_hash_verified": true}, nil
	}
	return nil, errors.Join(append([]error{errors.New("reported conclusion matches no sealed and analyzed query")}, mismatches...)...)
}

func plan11OriginalPointEvidence(body []byte, host string) (plan11PointEvidence, error) {
	var value struct {
		Status string
		Meta   struct {
			Latest *time.Time `json:"latest_sample_at"`
		} `json:"argus_meta"`
		Data struct {
			ResultType string
			Result     any
		}
		QueryContext struct{ Resources []struct{ Type, ID string } } `json:"query_context"`
	}
	var out plan11PointEvidence
	if json.Unmarshal(body, &value) != nil || value.Status != "success" || len(value.QueryContext.Resources) != 1 || value.QueryContext.Resources[0].Type != "host" || value.QueryContext.Resources[0].ID != host {
		return out, errors.New("metric result escaped the exact requested host or failed")
	}
	if value.Meta.Latest == nil || value.Meta.Latest.IsZero() {
		return out, errors.New("query has no observed sample provenance")
	}
	add := func(raw any) error {
		pair, ok := raw.([]any)
		if !ok || len(pair) != 2 {
			return errors.New("invalid returned point")
		}
		seconds, ok := pair[0].(float64)
		if !ok || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return errors.New("invalid point timestamp")
		}
		at := time.UnixMilli(int64(math.Round(seconds * 1000))).UTC()
		if out.Count == 0 || at.Before(out.First) {
			out.First = at
		}
		if out.Count == 0 || at.After(out.Last) {
			out.Last = at
		}
		out.Count++
		return nil
	}
	if value.Data.ResultType == "scalar" {
		if err := add(value.Data.Result); err != nil {
			return out, err
		}
	} else {
		rows, ok := value.Data.Result.([]any)
		if !ok {
			return out, errors.New("metric result has no point array")
		}
		for _, raw := range rows {
			row, ok := raw.(map[string]any)
			if !ok {
				return out, errors.New("invalid series")
			}
			if value.Data.ResultType == "vector" {
				point := row["value"]
				if point == nil {
					point = row["histogram"]
				}
				if err := add(point); err != nil {
					return out, err
				}
			} else if value.Data.ResultType == "matrix" {
				for _, key := range []string{"values", "histograms"} {
					points, _ := row[key].([]any)
					for _, point := range points {
						if err := add(point); err != nil {
							return out, err
						}
					}
				}
			} else {
				return out, errors.New("unsupported metric result shape")
			}
		}
	}
	if out.Count == 0 {
		return out, errors.New("actual host metric query returned no evidence points")
	}
	return out, nil
}
func plan11MatchPointConclusion(c plan11Conclusion, w plan11PointEvidence) error {
	first, e1 := time.Parse(time.RFC3339Nano, c.FirstPointAt)
	last, e2 := time.Parse(time.RFC3339Nano, c.LastPointAt)
	if c.PointCount == nil || *c.PointCount != w.Count || e1 != nil || e2 != nil || first.UnixMilli() != w.First.UnixMilli() || last.UnixMilli() != w.Last.UnixMilli() {
		return errors.New("reported point count/time range disagrees with immutable result")
	}
	return nil
}
func plan11ReadActualQuery(raw, path string) bool {
	var calls []struct {
		plan11ProcessOutcome
		Tool, Status string
		Input        map[string]any
	}
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return false
	}
	for _, call := range calls {
		if call.Status != "succeeded" {
			continue
		}
		if call.Tool == "read" {
			p, _ := call.Input["path"].(string)
			if p == path || p == strings.TrimPrefix(path, "/workspace/") {
				return true
			}
		}
		if call.Tool == "bash" && call.success() {
			command, _ := call.Input["command"].(string)
			if plan11CommandReferencesFile(command, path) {
				return true
			}
		}
	}
	return false
}

// Workspace commands start in /workspace. Accept the ordinary relative and
// explicit cd forms too; result identity/math are checked independently.
func plan11CommandReferencesFile(command, path string) bool {
	return plan11WorkspaceFileReference(command, path)
}

func plan11ExactHostArguments(arguments map[string]any, host string) bool {
	if raw, ok := arguments["scope"]; ok {
		if _, legacy := arguments["resource_ids"]; legacy {
			return false
		}
		scope, ok := raw.(map[string]any)
		if !ok || scope["mode"] != "explicit" {
			return false
		}
		refs, ok := scope["resources"].([]any)
		if !ok || len(refs) != 1 {
			return false
		}
		ref, ok := refs[0].(map[string]any)
		return ok && ref["type"] == "host" && ref["id"] == host
	}
	ids, ok := arguments["resource_ids"].([]any)
	return ok && len(ids) == 1 && ids[0] == host
}
