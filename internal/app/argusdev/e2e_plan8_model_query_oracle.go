package argusdev

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type plan8ModelToolFact struct {
	Tool, Status string
	Input        struct {
		Category, Name string
		Arguments      map[string]any
	}
}

// Public Tool Gateway identities are metric/query, log/query and trace/query.
// Internal MCP IDs such as telemetry.promql.query are not discovery identities.
func plan8VerifyApplicationModelQueries(raw string, apps []uuid.UUID, shared uuid.UUID) error {
	for _, category := range []string{"metric", "log", "trace"} {
		if err := verifyTypedToolDiscoveryOrder(raw, category); err != nil {
			return err
		}
	}
	var calls []plan8ModelToolFact
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return errors.New("application query evidence unavailable")
	}
	apm := map[uuid.UUID]bool{}
	logs, metric := false, false
	for _, call := range calls {
		if call.Tool != "tool.invoke" || call.Status != "succeeded" || call.Input.Name != "query" {
			continue
		}
		for _, app := range apps {
			if plan8ModelArgumentHasResource(call.Input.Arguments, app) {
				if call.Input.Category == "trace" {
					query, _ := call.Input.Arguments["document"].(string)
					view, _ := call.Input.Arguments["application_view"].(string)
					if strings.Contains(query, "queryAPM") || view == "services" || view == "instances" || view == "endpoints" || view == "red" || view == "topology" {
						apm[app] = true
					}
				}
				if call.Input.Category == "log" {
					logs = true
				}
			}
		}
		if call.Input.Category == "metric" && plan8ModelArgumentHasResource(call.Input.Arguments, shared) {
			metric = true
		}
	}
	if len(apm) != len(apps) || !logs || !metric {
		return errors.New("model did not query both application APM scopes, business logs and shared database")
	}
	return nil
}

func plan8VerifyModelMetricTarget(raw string, target uuid.UUID) error {
	if err := verifyTypedToolDiscoveryOrder(raw, "metric"); err != nil {
		return err
	}
	var calls []plan8ModelToolFact
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return errors.New("model metric tool facts unavailable")
	}
	for _, call := range calls {
		if call.Tool == "tool.invoke" && call.Status == "succeeded" && call.Input.Category == "metric" && call.Input.Name == "query" && plan8ModelArgumentHasResource(call.Input.Arguments, target) {
			return nil
		}
	}
	return errors.New("model did not query actual target metrics")
}

func plan8ModelArgumentHasResource(arguments map[string]any, id uuid.UUID) bool {
	values, ok := arguments["resource_ids"].([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if value == id.String() {
			return true
		}
	}
	return false
}
