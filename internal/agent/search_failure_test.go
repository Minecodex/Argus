package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestModelReceivesOnlyExplicitSearchRecovery(t *testing.T) {
	const code = "TOOL_SEARCH_REQUIRED"
	safe := toolruntime.Error{Kind: code, Message: "Search the category before requesting its exact Schema.", Details: map[string]any{"requires_search": true, "category": "metric", "name": "query"}}
	data := modelToolFailure(safe, code)
	if data["message"] != safe.Message || data["details"] == nil {
		t.Fatal("Search precondition recovery was lost")
	}
	for _, cause := range []error{errors.New("private storage information"), toolruntime.Error{Kind: code, Message: "private schema state"}} {
		raw, _ := json.Marshal(modelToolFailure(cause, code))
		if strings.Contains(string(raw), "private") {
			t.Fatal("unclassified error text entered the model context")
		}
	}
}
