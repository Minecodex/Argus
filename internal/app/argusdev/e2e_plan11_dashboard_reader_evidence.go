package argusdev

import (
	"encoding/json"
	"errors"
)

// Dashboard targets expose a direct series array. This is the same contract
// consumed by the portal; it is distinct from metric/query's PromQL envelope.
func plan11ReadOnlyDashboardSeries(execution map[string]any, host string) (int, error) {
	var value struct {
		Resources []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"resources"`
		Panels []struct {
			Targets []struct {
				Status string          `json:"status"`
				Kind   string          `json:"result_type"`
				Data   json.RawMessage `json:"data"`
			} `json:"targets"`
		} `json:"panels"`
	}
	raw, err := json.Marshal(execution)
	if err != nil || json.Unmarshal(raw, &value) != nil {
		return 0, errors.New("invalid dashboard execution response")
	}
	if len(value.Resources) != 1 || value.Resources[0].Type != "host" || value.Resources[0].ID != host {
		return 0, errors.New("read-only default query lacks its exact authorized Host scope")
	}
	series := 0
	for _, panel := range value.Panels {
		for _, target := range panel.Targets {
			if target.Status != "success" || target.Kind != "matrix" && target.Kind != "vector" {
				continue
			}
			var rows []struct {
				Labels map[string]string   `json:"metric"`
				Values [][]json.RawMessage `json:"values"`
				Value  []json.RawMessage   `json:"value"`
			}
			if json.Unmarshal(target.Data, &rows) != nil {
				return 0, errors.New("dashboard target data must be a direct series array")
			}
			if len(rows) == 0 {
				continue
			}
			facts, _, err := plan11PanelOriginalFacts(target.Data)
			if err != nil || facts.Count == 0 {
				return 0, errors.New("read-only default target has no valid numeric observations")
			}
			for _, row := range rows {
				if label, present := row.Labels["argus_resource_id"]; present && label != host {
					return 0, errors.New("read-only default query escaped its Host grant")
				}
				if len(row.Values) != 0 || len(row.Value) != 0 {
					series++
				}
			}
		}
	}
	if series == 0 {
		return 0, errors.New("read-only default query has no actual observed series")
	}
	return series, nil
}
