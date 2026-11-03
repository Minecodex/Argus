package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/interaction"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func plan11AmbiguityPrompt(locale string) string {
	if locale == "zh" {
		return fmt.Sprintf("请查看名为 %q 的 MySQL 监控目标当前采集配置和可用能力。先确认具体对象，存在歧义时向我询问；不要自行选第一条，不修改配置、不做实时或历史数据查询。确认后用 JSON 代码块回答 resource_id、collection_enabled、sampling_performed。请用中文回答。", plan11AmbiguousTargetName)
	}
	return fmt.Sprintf("Inspect the current collection configuration and available capabilities of the MySQL monitoring target named %q. Resolve the exact object first and ask me if it is ambiguous; do not pick the first row. Do not change configuration or query telemetry. After resolution, answer with a JSON block containing resource_id, collection_enabled, sampling_performed. Answer in English.", plan11AmbiguousTargetName)
}

func (a *App) plan11NavigationIdentityObserved(ctx context.Context, env *E2EEnvironment, run string, id uuid.UUID) error {
	value, err := a.postgresQuery(ctx, env, `SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id
 WHERE t.run_id='`+run+`' AND t.status='succeeded' AND t.tool_id='tool.invoke'
 AND ((t.input->>'category'='observe' AND t.input->>'name' IN ('resources.list','capabilities.get'))
 OR (t.input->>'category'='monitoring' AND t.input->>'name' IN ('target.list','collection.list','collection.get')))
 AND position('`+id.String()+`' in r.projection::text)>0`)
	if err != nil || strings.TrimSpace(value) == "0" || strings.TrimSpace(value) == "" {
		return errors.Join(err, errors.New("navigation did not observe the actual authorized resource identity"))
	}
	return nil
}

func (a *App) plan11ResolveAmbiguity(ctx context.Context, env *E2EEnvironment, conversation string, sample *p5BenchmarkSample, first, selected monitoring.Collection, proof map[string]any) error {
	return a.plan11ResolveAmbiguityWithFacts(ctx, env, conversation, sample, first, selected, proof,
		[]string{"metadata-a.invalid"}, []string{"metadata-b.invalid"}, "Use only monitoring target "+selected.Target.ID.String()+", endpoint metadata-b.invalid:3306. Inspect configuration only; do not query telemetry or make changes.")
}

func (a *App) plan11ResolveAmbiguityWithFacts(ctx context.Context, env *E2EEnvironment, conversation string, sample *p5BenchmarkSample, first, selected monitoring.Collection, proof map[string]any, firstFacts, selectedFacts []string, answer string) error {
	if sample.RunStatus != "waiting_input" {
		return errors.New("same-name target was not resolved through a real user question")
	}
	for _, id := range []uuid.UUID{first.Target.ID, selected.Target.ID} {
		if err := a.plan11NavigationIdentityObserved(ctx, env, sample.RunID, id); err != nil {
			return err
		}
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	value, err := client.JSON(ctx, "plan11-navigation-questions", "enterprise", http.MethodGet, "/conversations/"+conversation+"/interactions", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(value)
	var page struct{ Items []interaction.View }
	if json.Unmarshal(raw, &page) != nil {
		return errors.New("invalid navigation interaction list")
	}
	var question *interaction.View
	for i := range page.Items {
		v := &page.Items[i]
		if v.RunID.String() == sample.RunID && v.Status == "pending" && v.Kind == "user_input" {
			if question != nil {
				return errors.New("navigation produced more than one pending clarification")
			}
			question = v
		}
	}
	if question == nil {
		return errors.New("same-name task has no durable user question")
	}
	var questions interaction.Questions
	if json.Unmarshal(question.Content, &questions) != nil || len(questions.Questions) != 1 {
		return errors.New("same-name task added unrelated questions")
	}
	candidates := []uuid.UUID{first.Target.ID, selected.Target.ID}
	if !plan11QuestionIdentifies(string(question.Content), first.Target.ID, firstFacts, candidates...) || !plan11QuestionIdentifies(string(question.Content), selected.Target.ID, selectedFacts, candidates...) {
		return errors.New("same-name question did not provide actual distinguishing resource facts")
	}
	// Free text is the normal supported answer path. It selects an exact ID,
	// rather than guessing how the model labelled its recommended option.
	response := interaction.Response{Version: question.Version, ContentHash: question.ContentHash, Action: "answer", Answers: map[string]interaction.Answer{questions.Questions[0].ID: {Text: answer}}}
	if _, err = client.JSON(ctx, "plan11-navigation-answer", "enterprise", http.MethodPost, "/conversations/"+conversation+"/interactions/"+question.ID.String()+"/respond", 200, response, enterpriseHeaders(env, "plan11-navigation-answer-"+question.ID.String())); err != nil {
		return err
	}
	if err = a.waitRunStatus(ctx, env, sample.RunID, "succeeded", 4*time.Minute); err != nil {
		return err
	}
	final, err := a.p5BenchmarkSample(ctx, env, sample.ID, sample.RunID)
	if err != nil {
		return err
	}
	*sample = final
	reply, err := a.planV2ModelReply(ctx, env, sample.RunID)
	if err != nil {
		return err
	}
	var conclusion struct {
		Resource string `json:"resource_id"`
		Enabled  *bool  `json:"collection_enabled"`
		Checked  *bool  `json:"sampling_performed"`
	}
	start := strings.LastIndex(reply, "```json")
	if start < 0 {
		return errors.New("resolved target omitted its structured configuration facts")
	}
	text := reply[start+7:]
	end := strings.Index(text, "```")
	if end < 0 || json.Unmarshal([]byte(strings.TrimSpace(text[:end])), &conclusion) != nil || conclusion.Resource != selected.Target.ID.String() || conclusion.Enabled == nil || *conclusion.Enabled || conclusion.Checked == nil || *conclusion.Checked {
		return errors.New("resolved target identity/configuration or sampling claim differs from actual fixture facts")
	}
	observed, err := a.postgresQuery(ctx, env, plan11NavigationStateReadSQL(sample.RunID, selected))
	if err != nil || strings.TrimSpace(observed) == "0" {
		return errors.Join(err, errors.New("resolved collection state has no actual capability or collection read"))
	}
	proof["clarification_id"], proof["selected_resource"], proof["conclusion"] = question.ID, selected.Target, conclusion
	return nil
}

func plan11QuestionIdentifies(content string, id uuid.UUID, facts []string, candidates ...uuid.UUID) bool {
	if strings.Contains(strings.ToLower(content), id.String()) {
		return true
	}
	for _, fact := range facts {
		if fact != "" && strings.Contains(content, fact) {
			return true
		}
	}
	return plan11UniqueQuestionAbbreviation(content, id, candidates)
}

func plan11NavigationTraceAllowed(raw string) error {
	var calls []struct {
		Tool  string
		Input struct{ Category, Name string }
	}
	if json.Unmarshal([]byte(raw), &calls) != nil || len(calls) == 0 {
		return errors.New("navigation trace unavailable")
	}
	for _, call := range calls {
		switch call.Tool {
		case "tool.search", "tool.describe", "request_user_input", "read":
		case "tool.invoke":
			allowed := call.Input.Category == "observe" && (call.Input.Name == "resources.list" || call.Input.Name == "capabilities.get")
			allowed = allowed || call.Input.Category == "monitoring" && (call.Input.Name == "target.list" || call.Input.Name == "collection.list" || call.Input.Name == "collection.get" || call.Input.Name == "integration.list" || call.Input.Name == "collector.choices")
			if !allowed {
				return errPlan11NavigationBoundary
			}
		default:
			return errPlan11NavigationBoundary
		}
	}
	return nil
}

// A successful directory row is as authoritative as a detail read. Match the
// exact collection and typed target plus its actual configured state.
func plan11NavigationStateReadSQL(run string, selected monitoring.Collection) string {
	return `SELECT count(*) FROM tool_calls t LEFT JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id WHERE t.run_id='` + run + `'
 AND t.status='succeeded' AND t.tool_id='tool.invoke' AND
 ((t.input->>'category'='observe' AND t.input->>'name'='capabilities.get' AND position('` + selected.Target.ID.String() + `' in (t.input->'arguments')::text)>0)
 OR (t.input->>'category'='monitoring' AND t.input->>'name'='collection.get' AND position('` + selected.ID.String() + `' in (t.input->'arguments')::text)>0)
 OR (t.input->>'category'='monitoring' AND t.input->>'name'='collection.list' AND EXISTS (
 SELECT 1 FROM jsonb_array_elements(coalesce(r.projection->'summary'->'items','[]'::jsonb)) item
 WHERE item->>'id'='` + selected.ID.String() + `' AND item->'target'->>'kind'='monitoring_target'
 AND item->'target'->>'id'='` + selected.Target.ID.String() + `' AND item->>'desired_enabled'='false'
 AND item->>'integration'='mysql')))`
}
