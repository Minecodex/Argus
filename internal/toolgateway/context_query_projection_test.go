package toolgateway

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"testing"
)

func TestCompactQueryViewRetainsFullOriginalAndExactFiles(t *testing.T) {
	full := map[string]any{"id": uuid.New().String(), "status": "complete", "files": []any{map[string]any{"path": "/workspace/queries/full-opaque-id/manifest.json"}}, "manifest": map[string]any{"definition": "full source content"}}
	before, _ := json.Marshal(full)
	result := mcp.Result{Structured: full, Private: map[string]any{"model_summary": map[string]any{"id": full["id"], "status": "complete", "files": full["files"]}}}
	registry := mcp.NewRegistry()
	if err := (DashboardTools{Service: dashboard.Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	metadata, ok := registry.Lookup("telemetry.dashboard.query.get")
	if !ok || metadata.Project == nil {
		t.Fatal("native query projection was not registered")
	}
	preview := metadata.Project(result)
	if preview["manifest"] != nil || preview["files"] == nil || preview["status"] != "complete" {
		t.Fatal("task status or references lost")
	}
	after, _ := json.Marshal(result.Structured)
	if string(before) != string(after) {
		t.Fatal("original artifact changed")
	}
}
