package kubernetesinventory

import (
	"encoding/json"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sort"
	"strings"
	"time"
)

func enrichSummary(object Object, result *Summary) {
	var data map[string]any
	if json.Unmarshal(object.Data, &data) != nil {
		return
	}
	asMap := func(value any) map[string]any { v, _ := value.(map[string]any); return v }
	spec := asMap(data["spec"])
	status := asMap(data["status"])
	entries := func(value any) string {
		source := asMap(value)
		keys := []string{}
		for key := range source {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := []string{}
		for _, key := range keys {
			values = append(values, key+"="+fmt.Sprint(source[key]))
		}
		return strings.Join(values, ", ")
	}
	rows := func(value any) []any { v, _ := value.([]any); return v }
	switch object.Definition.Key {
	case "node":
		roles := []string{}
		for key := range object.Labels {
			if strings.HasPrefix(key, "node-role.kubernetes.io/") {
				roles = append(roles, strings.TrimPrefix(key, "node-role.kubernetes.io/"))
			}
		}
		if len(roles) == 0 {
			for key := range asMap(asMap(data["metadata"])["labels"]) {
				if strings.HasPrefix(key, "node-role.kubernetes.io/") {
					roles = append(roles, strings.TrimPrefix(key, "node-role.kubernetes.io/"))
				}
			}
		}
		sort.Strings(roles)
		if len(roles) > 0 {
			result.Fields["roles"] = strings.Join(roles, ", ")
		}
	case "service":
		result.Fields["selector"] = entries(spec["selector"])
	case "pdb":
		if spec["selector"] != nil {
			raw, _ := json.Marshal(spec["selector"])
			var selector metav1.LabelSelector
			if json.Unmarshal(raw, &selector) == nil {
				if value, err := metav1.LabelSelectorAsSelector(&selector); err == nil {
					result.Fields["selector"] = value.String()
					if value.Empty() {
						result.Fields["selector"] = "*"
					}
				}
			}
		}
	case "hpa":
		for _, source := range []struct{ key, field, value string }{{"metrics", "metric_targets", "target"}, {"currentMetrics", "metric_current", "current"}} {
			object := spec
			if source.value == "current" {
				object = status
			}
			values := []string{}
			for _, raw := range rows(object[source.key]) {
				metric := asMap(raw)
				typ, _ := metric["type"].(string)
				if typ == "" {
					continue
				}
				content := asMap(metric[strings.ToLower(typ[:1])+typ[1:]])
				name, _ := content["name"].(string)
				if name == "" {
					name, _ = asMap(content["metric"])["name"].(string)
				}
				val := asMap(content[source.value])
				for _, key := range []string{"averageUtilization", "averageValue", "value"} {
					if val[key] != nil {
						unit := ""
						if key == "averageUtilization" {
							unit = "%"
						}
						values = append(values, name+": "+fmt.Sprint(val[key])+unit)
						break
					}
				}
			}
			if len(values) > 0 {
				result.Fields[source.field] = strings.Join(values, "; ")
			}
		}
	case "ingress":
		paths := []string{}
		for _, raw := range rows(spec["rules"]) {
			rule := asMap(raw)
			host, _ := rule["host"].(string)
			for _, rawPath := range rows(asMap(rule["http"])["paths"]) {
				p := asMap(rawPath)
				path, _ := p["path"].(string)
				paths = append(paths, host+path)
			}
		}
		result.Fields["paths"] = strings.Join(paths, ", ")
		addresses := []string{}
		for _, raw := range rows(asMap(status["loadBalancer"])["ingress"]) {
			item := asMap(raw)
			for _, key := range []string{"ip", "hostname"} {
				if value, ok := item[key].(string); ok && value != "" {
					addresses = append(addresses, value)
				}
			}
		}
		result.Fields["addresses"] = strings.Join(addresses, ", ")
	case "role", "clusterrole":
		rules := []string{}
		for _, raw := range rows(data["rules"]) {
			rule := asMap(raw)
			rules = append(rules, fmt.Sprint(rule["apiGroups"])+" / "+fmt.Sprint(rule["resources"])+" / "+fmt.Sprint(rule["verbs"]))
		}
		result.Fields["rule_summary"] = strings.Join(rules, "; ")
	case "rolebinding", "clusterrolebinding":
		subjects := []string{}
		for _, raw := range rows(data["subjects"]) {
			subject := asMap(raw)
			subjects = append(subjects, fmt.Sprint(subject["kind"])+"/"+fmt.Sprint(subject["name"]))
		}
		result.Fields["subject_names"] = strings.Join(subjects, ", ")
	case "job":
		start, e := time.Parse(time.RFC3339, fmt.Sprint(status["startTime"]))
		if e == nil {
			end := object.ObservedAt
			if finished, e := time.Parse(time.RFC3339, fmt.Sprint(status["completionTime"])); e == nil {
				end = finished
			}
			if !end.IsZero() && !end.Before(start) {
				result.Fields["duration_seconds"] = fmt.Sprintf("%.0f", end.Sub(start).Seconds())
			}
		}
	case "endpointslice":
		ready, unknown := 0, 0
		for _, raw := range rows(data["endpoints"]) {
			conditions := asMap(asMap(raw)["conditions"])
			if conditions["ready"] == true {
				ready++
			}
			if conditions["ready"] == nil {
				unknown++
			}
		}
		result.Fields["ready_endpoints"] = fmt.Sprintf("%d / %d", ready, len(rows(data["endpoints"])))
		if unknown > 0 {
			result.Fields["unknown_endpoints"] = fmt.Sprint(unknown)
		}
	}
}
