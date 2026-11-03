package argusdev

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kakj-go/Argus/internal/dashboard"
)

// Inspect every available execution before ordinary completion gates. A
// narrowed/empty/incomplete query must not hide a later out-of-scope query.
func plan11ComparisonReadBoundaries(h *plan6HostLoop, window plan11ComparisonWindow, question string, calls []plan8ModelToolFact, files []plan11QueryFile, jobs []dashboard.QueryJobView) error {
	var failures []error
	for _, call := range calls {
		if call.Status == "succeeded" && call.Tool == "tool.invoke" && call.Input.Category == "metric" && call.Input.Name == "query" {
			if err := plan11ComparisonQueryConstraint(call.Input.Arguments, h.HostID, window); err != nil {
				failures = append(failures, err)
			}
		}
	}
	for _, file := range files {
		var value struct {
			Data struct {
				Rows []struct {
					Labels map[string]string `json:"metric"`
				} `json:"result"`
			} `json:"data"`
		}
		if json.Unmarshal(file.Body, &value) != nil {
			continue // Numeric/integrity gates separately reject unavailable data.
		}
		for _, row := range value.Data.Rows {
			if id := row.Labels["argus_resource_id"]; id != "" && id != h.HostID {
				failures = append(failures, fmt.Errorf("%w: original metric result contains another Host", errPlan11ComparisonBoundary))
			}
		}
	}
	for _, job := range jobs {
		if question == "dashboard" && job.DashboardID.String() != h.DashboardID {
			failures = append(failures, fmt.Errorf("%w: query selected another dashboard", errPlan11ComparisonBoundary))
		}
		if job.Manifest == nil {
			failures = append(failures, errors.New("comparison dashboard manifest unavailable"))
			continue
		}
		execution := job.Manifest.Execution
		if len(execution.Resources) == 0 {
			failures = append(failures, errors.New("comparison dashboard did not select its requested Host"))
		}
		for _, resource := range execution.Resources {
			if resource.Type != "host" || resource.ID.String() != h.HostID {
				failures = append(failures, fmt.Errorf("%w: dashboard execution escaped the fixed Host", errPlan11ComparisonBoundary))
			}
		}
		if err := plan11ComparisonWindowConstraint(execution.From, execution.To, window); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
