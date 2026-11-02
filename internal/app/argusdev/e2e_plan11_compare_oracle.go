package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

var errPlan11ComparisonBoundary = errors.New("comparison evidence boundary failed")

// Legacy files have neither query_context nor latest_sample_at. The common
// oracle uses exact recorded inputs, retained identity labels, actual loaded
// samples and sealed file bytes instead of requiring candidate-only fields.
func plan11ComparisonPointEvidence(body []byte, host string) (plan11PointEvidence, error) {
	var value struct {
		Status string
		Meta   struct {
			Loaded int64 `json:"loaded_samples"`
		} `json:"argus_meta"`
		Data struct {
			Kind string `json:"resultType"`
			Rows []struct {
				Labels map[string]string   `json:"metric"`
				Values [][]json.RawMessage `json:"values"`
				Value  []json.RawMessage   `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	var out plan11PointEvidence
	if json.Unmarshal(body, &value) != nil || value.Status != "success" || value.Meta.Loaded < 1 || (value.Data.Kind != "matrix" && value.Data.Kind != "vector") || len(value.Data.Rows) == 0 {
		return out, errors.New("comparison query lacks actual measured numeric series")
	}
	for _, row := range value.Data.Rows {
		if id := row.Labels["argus_resource_id"]; id != "" && id != host {
			return out, fmt.Errorf("%w: output escaped the exact Host", errPlan11ComparisonBoundary)
		}
		if row.Labels["argus_resource_id"] == "" {
			return out, errors.New("comparison result has no retained resource identity; aggregation is not proof of another Host")
		}
		points := row.Values
		if value.Data.Kind == "vector" {
			points = [][]json.RawMessage{row.Value}
		}
		for _, point := range points {
			var seconds float64
			if len(point) != 2 || json.Unmarshal(point[0], &seconds) != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
				return out, errors.New("comparison point timestamp invalid")
			}
			at := time.UnixMilli(int64(math.Round(seconds * 1000))).UTC()
			if out.Count == 0 || at.Before(out.First) {
				out.First = at
			}
			if out.Count == 0 || at.After(out.Last) {
				out.Last = at
			}
			out.Count++
		}
	}
	if out.Count < 1 {
		return out, errors.New("comparison query returned no points")
	}
	return out, nil
}

func (a *App) plan11VerifyComparisonQuery(ctx context.Context, env *E2EEnvironment, conversation, run, host, calls string, c plan11Conclusion) (map[string]any, error) {
	if uuid.Validate(run) != nil || uuid.Validate(host) != nil {
		return nil, errors.New("comparison identity invalid")
	}
	var units map[string]string
	if json.Unmarshal([]byte(env.State.Values["plan11_comparison_units"]), &units) != nil {
		return nil, errors.New("comparison reference units unavailable")
	}
	unit, ok := units[c.Metric]
	if !ok {
		return nil, errors.New("comparison metric absent from the frozen reference catalog")
	}
	return a.plan11VerifyMeasuredQuery(ctx, env, conversation, run, host, calls, c, unit, plan11MatchNumericConclusion)
}

func (a *App) plan11VerifyMeasuredQuery(ctx context.Context, env *E2EEnvironment, conversation, run, resource, calls string, c plan11Conclusion, unit string, match func(plan11Conclusion, string, plan11ValueEvidence) error) (map[string]any, error) {
	if uuid.Validate(run) != nil || uuid.Validate(resource) != nil {
		return nil, errors.New("invalid measured query identity")
	}
	rows, err := a.plan11LoadQueryFiles(ctx, env, conversation, run)
	if err != nil {
		return nil, err
	}
	var failures []error
	for _, row := range rows {
		if !strings.Contains(row.Query, c.Metric) || row.FileID == nil {
			continue
		}
		id, err := uuid.Parse(row.ID)
		if err != nil {
			return nil, err
		}
		body := row.Body
		points, err := plan11ComparisonPointEvidence(body, resource)
		if err == nil {
			err = plan11MatchPointConclusion(c, points)
		}
		var values plan11ValueEvidence
		if err == nil {
			values, err = plan11OriginalValueEvidence(body, c.Metric)
		}
		if err == nil {
			err = match(c, unit, values)
		}
		path := "/workspace/" + toolruntime.NativeQueryFile("telemetry.promql.query", id)
		if err == nil && (!plan11ReadActualQuery(calls, path) || !plan11ComputedStatistics(calls, path)) {
			err = errors.New("comparison did not analyze its actual query file")
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		var envelope struct {
			Context map[string]any `json:"query_context"`
		}
		if err = json.Unmarshal(body, &envelope); err != nil {
			return nil, err
		}
		return map[string]any{"query_call_id": id, "content_hash": row.Hash, "returned_points": points, "returned_values": values, "unit": unit, "query_context": envelope.Context, "oracle": "version-neutral sealed input and identity-label evidence"}, nil
	}
	return nil, errors.Join(append([]error{errors.New("comparison conclusion matches no measured and analyzed query")}, failures...)...)
}
