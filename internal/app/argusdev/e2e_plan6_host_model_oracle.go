package argusdev

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Compare the actual model conclusion to its authorized tool observations,
// rather than treating tool presence or a successful Run as semantic proof.
func plan6HostVerifyAlertConclusion(reply, trace, host, policy, phase string) error {
	type item struct {
		ID, Lifecycle string
		Value         *float64
		Kind          string
	}
	var conclusion struct {
		HostID    string `json:"host_id"`
		Instances []item `json:"instances"`
	}
	blocks := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```").FindAllStringSubmatch(reply, -1)
	if len(blocks) == 0 || json.Unmarshal([]byte(strings.TrimSpace(blocks[len(blocks)-1][1])), &conclusion) != nil || conclusion.HostID != host || len(conclusion.Instances) < 2 {
		return fmt.Errorf("GLM did not summarize both actual Host CPU alert instances")
	}
	var calls []struct {
		Tool, Status string
		Output       map[string]any
	}
	if err := json.Unmarshal([]byte(trace), &calls); err != nil {
		return err
	}
	expectedLife := "firing"
	if phase == "recovered" {
		expectedLife = "resolved"
	}
	observed := map[string][]item{}
	for _, call := range calls {
		if call.Tool != "tool.invoke" || call.Status != "succeeded" {
			continue
		}
		page, _ := call.Output["page"].(map[string]any)
		records := objectItems(page)
		if detail, ok := call.Output["instance"].(map[string]any); ok {
			records = append(records, detail)
		}
		for _, record := range records {
			if record["policy_id"] != policy {
				continue
			}
			resources, _ := record["resource_ids"].([]any)
			if len(resources) != 1 || resources[0] != host {
				return fmt.Errorf("Host alert conclusion received an out-of-scope instance")
			}
			state, _ := record["state"].(map[string]any)
			id, _ := record["id"].(string)
			life, _ := record["lifecycle"].(string)
			kind, _ := state["kind"].(string)
			var value *float64
			if n, ok := state["value"].(float64); ok {
				value = &n
			}
			observed[id] = append(observed[id], item{ID: id, Lifecycle: life, Value: value, Kind: kind})
		}
	}
	seen := map[string]bool{}
	conditionCount := 0
	for _, stated := range conclusion.Instances {
		if seen[stated.ID] {
			return fmt.Errorf("GLM reported duplicate or incorrect Host alert lifecycle")
		}
		seen[stated.ID] = true
		match := false
		condition := false
		for _, actual := range observed[stated.ID] {
			valueMatches := stated.Value == nil && actual.Value == nil || stated.Value != nil && actual.Value != nil && math.Abs(*stated.Value-*actual.Value) <= 1e-6
			match = match || stated.Lifecycle == actual.Lifecycle && valueMatches
			condition = condition || actual.Kind == "condition" && actual.Lifecycle == expectedLife
		}
		if !match {
			return fmt.Errorf("GLM Host alert value/identity differs from the queried instance %s: stated=%v observed=%v", stated.ID, stated.Value, observed[stated.ID])
		}
		if condition {
			conditionCount++
		}
	}
	if conditionCount != 2 || len(seen) != len(observed) {
		return fmt.Errorf("GLM omitted queried Host instances or the two %s conditions", expectedLife)
	}
	return nil
}
