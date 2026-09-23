package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Drive native discovery through the real Agent, persisted ToolResults and
// model-facing projections. No business mutation is needed for these checks.
func (a *App) verifyP5BusinessDiscovery(ctx context.Context, env *E2EEnvironment) error {
	cases := []struct{ category, query, name string }{
		{"host", "创建", "create.preview"}, {"host", "列表", "list"}, {"host", "删除", "delete.preview"},
		{"k8s", "命名空间", "namespace.list"}, {"metric", "CPU", "query"}, {"log", "错误日志", "query"}, {"trace", "慢调用", "query"},
		{"connector", "清单", "list"}, {"workflow", "发布 下载", "publish_file"},
	}
	for i, tc := range cases {
		convo, err := a.p5Conversation(ctx, env, fmt.Sprintf("business-discovery-%d", i))
		if err != nil {
			return err
		}
		run, err := a.p5Run(ctx, env, convo, fmt.Sprintf("discovery-%d", i), []p5ToolStep{
			{"tool.search", map[string]any{"category": tc.category, "query": tc.query}},
			{"tool.describe", map[string]any{"category": tc.category, "name": tc.name}},
		}, []string{})
		if err != nil {
			return err
		}
		value, err := a.postgresQuery(ctx, env, `SELECT json_build_object(
   'searches',(SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id WHERE t.run_id='`+run+`' AND t.tool_id='tool.search' AND t.status='succeeded' AND EXISTS(SELECT 1 FROM jsonb_array_elements(r.projection->'summary'->'items') item WHERE item->>'name'='`+tc.name+`')),
   'descriptions',(SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id WHERE t.run_id='`+run+`' AND t.tool_id='tool.describe' AND t.status='succeeded' AND r.projection->'summary'->>'schema_version'='argus.tool_manifest/v1' AND length(r.projection->'summary'->>'discovery_hash')=64 AND length(r.projection->'summary'->>'result_description')>0 AND jsonb_array_length(r.projection->'summary'->'keywords')>0 AND jsonb_array_length(r.projection->'summary'->'examples')>0 AND jsonb_array_length(r.projection->'summary'->'preconditions')>0),
   'business_mutations',(SELECT count(*) FROM pending_actions WHERE run_id='`+run+`'));
`)
		if err != nil {
			return err
		}
		var proof struct {
			Searches     int `json:"searches"`
			Descriptions int `json:"descriptions"`
			Mutations    int `json:"business_mutations"`
		}
		if err = json.Unmarshal([]byte(strings.TrimSpace(value)), &proof); err != nil {
			return err
		}
		if proof.Searches != 1 || proof.Descriptions != 1 || proof.Mutations != 0 {
			return fmt.Errorf("P5 business discovery %s/%s: %s", tc.category, tc.query, value)
		}
	}
	data, _ := json.Marshal(map[string]any{"categories": 7, "business_queries": len(cases), "persisted_search_hits": len(cases), "complete_descriptions": len(cases), "business_mutations": 0, "fixture": "deterministic replay; not real model evaluation"})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-business-discovery.json"), append(data, '\n'))
}
