package argusdev

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type p5BenchmarkSample struct {
	ID                     string         `json:"id"`
	RunID                  string         `json:"run_id"`
	RunStatus              string         `json:"run_status"`
	Passed                 bool           `json:"passed"`
	InputTokens            int64          `json:"input_tokens"`
	OutputTokens           int64          `json:"output_tokens"`
	TotalTokens            int64          `json:"total_tokens"`
	CachedInputTokens      int64          `json:"cached_input_tokens"`
	CachedUsageComplete    bool           `json:"cached_usage_complete"`
	UsageComplete          bool           `json:"usage_complete"`
	ModelCalls             int            `json:"model_calls"`
	InferenceCalls         int            `json:"inference_calls"`
	DurationMS             float64        `json:"duration_ms"`
	FirstEffectiveResultMS *float64       `json:"first_effective_result_ms"`
	ExternalCalls          int            `json:"external_calls"`
	NativeLists            map[string]int `json:"native_lists"`
}

func (a *App) p5BenchmarkSample(ctx context.Context, env *E2EEnvironment, id, run string) (p5BenchmarkSample, error) {
	if _, err := uuid.Parse(run); err != nil {
		return p5BenchmarkSample{}, fmt.Errorf("P5 benchmark received invalid run identity")
	}
	value, err := a.postgresQuery(ctx, env, p5BenchmarkSampleSQL(run))
	if err != nil {
		return p5BenchmarkSample{}, err
	}
	var sample p5BenchmarkSample
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &sample); err != nil {
		return sample, err
	}
	sample.ID = id
	return sample, nil
}

func p5BenchmarkSampleSQL(run string) string {
	return `SELECT json_build_object('run_id',r.id,'run_status',r.status,
 'input_tokens',coalesce(m.input_tokens,0),'output_tokens',coalesce(m.output_tokens,0),
 'total_tokens',coalesce(m.input_tokens,0)+coalesce(m.output_tokens,0),'usage_complete',m.usage_complete,
 'cached_input_tokens',coalesce(m.cached_input_tokens,0),'cached_usage_complete',m.cached_usage_complete,
 'model_calls',m.model_calls,'inference_calls',m.inference_calls,
 'duration_ms',round(extract(epoch FROM (r.updated_at-r.created_at))*1000),
 'first_effective_result_ms',(SELECT round(extract(epoch FROM (min(t.updated_at)-r.created_at))*1000) FROM tool_calls t
 WHERE t.run_id=r.id AND t.status='succeeded' AND t.tool_id NOT IN ('tool.search','tool.describe')),
 'external_calls',(SELECT count(*) FROM tool_calls t WHERE t.run_id=r.id AND t.source='external_mcp' AND t.status='succeeded'),
 'native_lists',(SELECT coalesce(json_object_agg(category,calls),'{}'::json) FROM
 (SELECT input->>'category' AS category,count(*) AS calls FROM tool_calls WHERE run_id=r.id
 AND status='succeeded' AND tool_id='tool.invoke'
 AND ((input->>'category' IN ('host','connector') AND input->>'name'='list')
 OR (input->>'category'='k8s' AND input->>'name'='cluster.list')) GROUP BY input->>'category') lists))
 FROM runs r CROSS JOIN LATERAL (SELECT sum(input_tokens) FILTER(WHERE input_usage_source='provider') AS input_tokens,sum(output_tokens) FILTER(WHERE output_usage_source='provider') AS output_tokens,
 sum(cached_input_tokens) FILTER(WHERE cached_input_usage_source='provider' AND input_usage_source='provider' AND cached_input_tokens<=input_tokens) AS cached_input_tokens,
 count(*)>0 AND bool_and(cached_input_usage_source='provider' AND input_usage_source='provider' AND cached_input_tokens<=input_tokens AND status='succeeded') AS cached_usage_complete,
 count(*) AS model_calls,count(*) FILTER(WHERE call_kind='inference') AS inference_calls,
 count(*)>0 AND count(*) FILTER(WHERE status='succeeded' AND input_usage_source='provider' AND output_usage_source='provider' AND cached_input_usage_source<>'invalid' AND (cached_input_usage_source<>'provider' OR cached_input_tokens<=input_tokens) AND EXISTS(SELECT 1 FROM model_quota_reservations q WHERE q.model_call_id=c.id AND q.enterprise_id=c.enterprise_id AND q.status='settled' AND q.usage_source='provider'))=count(*) AS usage_complete
 FROM model_calls c WHERE run_id=r.id) m
 WHERE r.id='` + run + `';`
}

func (a *App) p5BenchmarkDelivery(ctx context.Context, env *E2EEnvironment, conversation, run string, factor int) (bool, error) {
	if _, err := uuid.Parse(run); err != nil {
		return false, fmt.Errorf("P5 benchmark received invalid run identity")
	}
	ids, err := a.postgresQuery(ctx, env, "SELECT id::text FROM file_deliveries WHERE run_id='"+run+"' ORDER BY created_at;")
	if err != nil {
		return false, err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return false, err
	}
	for _, id := range strings.Fields(ids) {
		if _, err := uuid.Parse(id); err != nil {
			return false, fmt.Errorf("P5 benchmark received invalid delivery identity")
		}
		data, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+conversation+"/deliveries/"+id+"/content", nil, 200, "")
		if err != nil {
			return false, err
		}
		if p5SalesCSVValid(data, factor) {
			return true, nil
		}
	}
	return false, nil
}

func p5SalesCSVValid(data []byte, factor int) bool {
	if len(data) > 4096 || (factor != 1 && factor != 2) {
		return false
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	rows, err := reader.ReadAll()
	if err != nil || len(rows) != 3 || len(rows[0]) != 2 || rows[0][0] != "region" || rows[0][1] != "total" {
		return false
	}
	want := map[string]float64{"East": float64(30 * factor), "West": float64(10 * factor)}
	for _, row := range rows[1:] {
		if len(row) != 2 {
			return false
		}
		expected, exists := want[row[0]]
		actual, err := strconv.ParseFloat(row[1], 64)
		if !exists || err != nil || actual != expected {
			return false
		}
		delete(want, row[0])
	}
	return len(want) == 0
}
