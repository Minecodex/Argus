package presentation

import (
	"fmt"
	"strings"
)

type Binding struct {
	Entry, Mode string
	Reads       []string
}

func ToolBinding(id string) (Binding, error) {
	if !declaredTools[id] {
		return Binding{}, fmt.Errorf("tool %s has no presentation coverage declaration", id)
	}
	read := []string{"result.detail", "result.page", "resources.context"}
	entry := "resource-list"
	switch {
	case id == "pending_action.get":
		return Binding{Mode: "action"}, nil
	case id == "tool.search" || id == "tool.describe" || id == "telemetry.dashboard.catalog" || id == "telemetry.dashboard.catalog.resources" || id == "telemetry.dashboard.draft.create" || id == "telemetry.dashboard.draft.validate":
		return Binding{Mode: "trace"}, nil
	case id == "workflow.publish_file" || id == "workflow.import_result":
		return Binding{Mode: "file"}, nil
	case id == "tool.invoke" || id == "pending_action.cancel" || strings.HasSuffix(id, ".cancel") || strings.HasSuffix(id, ".resume") || strings.Contains(id, ".context.") || id == "telemetry.dashboard.draft.save":
		return Binding{Mode: "trace"}, nil
	case id == "tool.search":
		entry = "tool-search"
	case id == "tool.describe":
		entry = "tool-description"
	case strings.HasSuffix(id, ".preview"):
		entry = "change-preview"
	case strings.HasPrefix(id, "application.") || strings.HasPrefix(id, "monitoring.") || id == "kubernetes.object.get" || id == "kubernetes.operation.get" || id == "kubernetes.workbench.types":
		if strings.HasSuffix(id, ".get") {
			entry = "resource-detail"
		}
	case id == "telemetry.promql.query":
		entry = "metric-query"
		read = append(read, "metric.context")
	case id == "telemetry.kql.query":
		entry = "log-query"
	case id == "telemetry.skywalking.trace":
		entry = "trace-result"
	case id == "telemetry.overview":
		entry = "telemetry-summary"
		read = append(read, "runtime.status")
	case id == "kubernetes.pod.logs":
		entry = "pod-logs"
	case strings.HasPrefix(id, "telemetry.dashboard."):
		switch strings.TrimPrefix(id, "telemetry.dashboard.") {
		case "query", "query.get":
			entry = "dashboard-result"
			read = append(read, "dashboard.result")
		case "list":
			entry = "dashboard-list"
		case "draft.validate":
			entry = "dashboard-draft-validation"
		case "catalog", "catalog.resources":
			entry = "telemetry-catalog"
		default:
			entry = "dashboard-definition"
		}
	case strings.HasPrefix(id, "alert."):
		read = append(read, "alert.details")
		switch id {
		case "alert.event.query":
			entry = "alert-events"
		case "alert.event.get":
			entry = "alert-event-detail"
		case "alert.history.query":
			entry = "alert-history"
		case "alert.notification.query":
			entry = "alert-notifications"
		case "alert.policy.list":
			entry = "alert-policy-list"
		case "alert.policy.get":
			entry = "alert-policy-detail"
		default:
			entry = "alert-settings-list"
		}
	case strings.HasPrefix(id, "pending_action."):
		if strings.HasSuffix(id, ".get") {
			entry = "action-detail"
		} else {
			entry = "action-list"
		}
	case strings.HasPrefix(id, "telemetry.collector."):
		if strings.HasSuffix(id, ".get") {
			entry = "collector-detail"
		} else {
			entry = "collector-list"
		}
		read = append(read, "runtime.status")
	case strings.HasPrefix(id, "connector."):
		if strings.HasSuffix(id, ".get") {
			entry = "connector-detail"
		} else {
			entry = "connector-list"
		}
		read = append(read, "runtime.status")
	case id == "host.get":
		entry = "host-detail"
	case id == "host.list":
		entry = "host-list"
	case id == "kubernetes.cluster.get":
		entry = "cluster-detail"
	case id == "kubernetes.cluster.list":
		entry = "cluster-list"
	case strings.HasPrefix(id, "kubernetes."):
		entry = "kubernetes-resource-list"
	}
	return Binding{Entry: entry, Mode: "template", Reads: read}, nil
}

func BindData(tool, title string, value map[string]any) map[string]any {
	data := Observed(PublicSnapshot(value))
	binding, err := ToolBinding(tool)
	if err != nil {
		return data
	}
	data["view"], data["tool"], data["tool_title"] = binding.Entry, tool, title
	NormalizeItems(binding.Entry, data)
	return data
}
