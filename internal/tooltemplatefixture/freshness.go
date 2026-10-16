package tooltemplatefixture

import "time"

// Keep failures, values and timestamps in the formal Instance/State shape. A
// recent failed attempt must never make a retained observation look current.
func freshnessScenarios() map[string]Fixture {
	scenarios := map[string]Fixture{}
	for _, health := range []string{"normal", "error", "no_data", "authorization_denied", "unknown", "recovered"} {
		data := compositeScenarios()["composite-every"].Data
		instance := data["instance"].(map[string]any)
		state := instance["state"].(map[string]any)
		success := now.Add(-5 * time.Minute)
		instance["health"], instance["last_evaluated_at"] = health, now
		state["last_successful_at"], state["last_slot"] = success, now
		if health == "normal" || health == "recovered" {
			instance["health"], state["last_successful_at"] = "normal", now
		}
		if health == "unknown" {
			delete(instance, "health")
			delete(state, "last_successful_at")
		}
		scenarios["health-"+health] = Fixture{Tool: "alert.event.get", Data: data, Expected: []string{"97.25 %", "CPU member", "Memory member"}}
		if health == "error" {
			for _, view := range []struct{ name, tool string }{{"events", "alert.event.query"}, {"history", "alert.history.query"}} {
				row := object(instance)
				if view.name == "history" {
					row["lifecycle"], row["resolved_at"] = "resolved", now
				}
				scenarios["health-error-"+view.name] = Fixture{Tool: view.tool, Data: map[string]any{"page": map[string]any{"items": []any{row}, "complete": true}, "resources": data["resources"]}, Expected: []string{"CPU member", "Memory member"}}
			}
		}
	}
	for _, version := range []string{"known", "unknown", "empty"} {
		data := resourceFixture("connector.list", "connector-list")
		row := data["items"].([]any)[0].(map[string]any)
		expected := []string{"db-prod-connector"}
		switch version {
		case "known":
			row["software_version"], row["version"], row["connection_epoch"] = "v9.8.7", 42, 73
			expected = append(expected, "v9.8.7")
		case "unknown":
			row["software_version"] = ""
		case "empty":
			data["items"], expected = []any{}, []string{}
		}
		scenarios["version-"+version] = Fixture{Tool: "connector.list", Data: data, Expected: expected}
	}
	return scenarios
}
