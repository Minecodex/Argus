package dashboard

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func testPublishedParameters(t *testing.T, ctx context.Context, service Service, runtime Runtime, actor Actor, workflow actionservice.Service, original Spec, host, source uuid.UUID) {
	raw, _ := json.Marshal(original)
	spec, err := DecodeSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	binding := spec.Panels[0].SourceBinding
	spec.Variables = []Variable{
		{ID: "parent", Name: "parent", IncludeAll: true, Multiple: true, Default: Selection{Values: []string{"authorized evidence", "gone"}}, Query: CandidateQuery{Signal: "logs", SourceBinding: binding, Field: "body"}},
		{ID: "entry", Name: "entry", IncludeAll: true, Default: Selection{Values: []string{"authorized evidence"}}, Query: CandidateQuery{Signal: "logs", SourceBinding: binding, Field: "body", Filters: []Filter{{Field: "body", Operator: "=", Variable: "parent"}}}},
	}
	spec.Panels[0].LocalFilters = []LocalFilter{
		{ID: "level", Kind: "query", Default: Selection{Values: []string{"0"}}, Query: &CandidateQuery{Signal: "logs", SourceBinding: binding, Field: "severity_number", Filters: []Filter{{Field: "body", Operator: "=", Variable: "entry"}}}},
		{ID: "term", Kind: "text", Default: Selection{All: true}},
	}
	target := &spec.Panels[0].Targets[0]
	target.SourceDefinition.DSL = &DSL{Expression: `body = "$entry" AND severity_number = "$level" AND body = "$term"`, Pipeline: "limit 5"}
	target.ParameterBindings = []ParameterBinding{{Parameter: "entry", Variable: "entry"}, {Parameter: "level", LocalParameter: "level"}, {Parameter: "term", LocalParameter: "term"}}
	static := original.Panels[0]
	static.ID = "static"
	static.Layout.Y = 12
	spec.Panels = append(spec.Panels, static)
	raw, _ = json.Marshal(spec)
	draft, err := service.CreateDraft(ctx, actor, DraftInput{Name: "Parameterized logs", Spec: raw})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewPublish(ctx, actor, draft.ID, draft.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	confirmation, err := workflow.Confirm(ctx, actor.SubjectID.String(), uuid.NewString(), actor.EnterpriseID, 1, false, preview.Action.ActionRef, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	var published resource.ActionCommitResult
	extension := ActionExtension{}
	if err := runtime.Store.InTx(ctx, func(q *db.Queries) error {
		var e error
		published, e = service.Actions.ExecuteReady(ctx, q, confirmation.PendingAction, extension.RevalidateAction, extension.CommitAction)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{ResourceIDs: []uuid.UUID{host}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Variables["parent"].All || !result.VariableCandidates["parent"].Reset || result.Variables["entry"].All || result.LocalValues["logs"]["level"].All || result.Panels[0].Status != "success" {
		t.Fatalf("candidate cascade failed: %+v", result)
	}
	if len(result.VariableCandidates["parent"].Sources) != 1 || result.VariableCandidates["parent"].Sources[0].ID != source {
		t.Fatal("candidate expanded source scope")
	}
	_, revision, err := service.Get(ctx, actor, published.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := DecodeSpec(revision.Spec)
	if err != nil || saved.Variables[0].Default.All {
		t.Fatal("runtime changed published defaults")
	}
	changed, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{From: &result.From, To: &result.To, ResourceIDs: []uuid.UUID{host}, PanelIDs: []string{"logs"}, LocalValues: map[string]map[string]Selection{"logs": {"term": {Values: []string{"not in logs"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Panels) != 1 || changed.Panels[0].Status != "no_data" || changed.LocalValues["logs"]["term"].All || changed.ExecutionHash == result.ExecutionHash {
		t.Fatalf("local refresh/reset/hash failed: %+v", changed)
	}
	other, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{PanelIDs: []string{"static"}, Variables: map[string]Selection{"entry": {All: true}}, LocalValues: map[string]map[string]Selection{"logs": {"term": {Values: []string{"not in logs"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Panels) != 1 || other.Panels[0].ID != "static" || other.Panels[0].Status != "success" || len(other.LocalCandidates["logs"]) != 0 {
		t.Fatalf("local filter affected unrelated panel: %+v", other)
	}
	candidates, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{From: &result.From, To: &result.To, ResourceIDs: []uuid.UUID{host}, CandidatesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates.Panels) != 0 || len(candidates.LocalCandidates["logs"]) != 0 || len(candidates.LocalCandidates["static"]) != 0 || !candidates.Variables["parent"].All || !candidates.VariableCandidates["parent"].Reset || candidates.VariableCandidates["entry"].Status != "success" {
		t.Fatalf("candidate reconciliation executed panels or lost cascade: %+v", candidates)
	}
}
