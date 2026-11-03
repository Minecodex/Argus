package argusdev

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
)

// Accept the equivalent explicit metric envelope without defaulting missing
// facts or silently choosing between contradictory summaries.
func plan11DecodeConclusion(raw []byte) (plan11Conclusion, error) {
	var result plan11Conclusion
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return result, errors.New("invalid conclusion object")
	}
	metric, nested := fields["metric_evidence"]
	if !nested {
		return result, json.Unmarshal(raw, &result)
	}
	if bytes.Equal(bytes.TrimSpace(metric), []byte("null")) {
		return result, errors.New("metric evidence envelope is null")
	}
	if bytes.HasPrefix(bytes.TrimSpace(metric), []byte("[")) {
		var entries []json.RawMessage
		if err := json.Unmarshal(metric, &entries); err != nil || len(entries) != 1 || !bytes.HasPrefix(bytes.TrimSpace(entries[0]), []byte("{")) {
			return result, errors.New("metric evidence array must contain exactly one object")
		}
		metric = entries[0]
	}
	for _, name := range []string{"resource_id", "metric", "point_count", "first_point_at", "last_point_at", "unit", "value_min", "value_max", "display_unit", "display_transform", "display_min", "display_max", "limitations"} {
		if _, exists := fields[name]; exists {
			return result, errors.New("conclusion contains both flat and nested metric evidence")
		}
	}
	if err := json.Unmarshal(metric, &result); err != nil {
		return result, err
	}
	if dashboards, exists := fields["dashboard_evidence"]; exists {
		var outer []plan11DashboardClaim
		if err := json.Unmarshal(dashboards, &outer); err != nil {
			return result, err
		}
		if len(result.Dashboards) != 0 && !reflect.DeepEqual(result.Dashboards, outer) {
			return result, errors.New("conflicting dashboard evidence in the conclusion envelope")
		}
		result.Dashboards = outer
	}
	return result, nil
}
