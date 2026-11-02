package argusdev

import (
	"errors"
	"fmt"
	"strings"
)

func plan8SelectedModelNames(phase string) []string {
	if plan11SignalPhase(phase) {
		return []string{"plan11-signal-general-zh", "plan11-signal-semantics-zh", "plan11-signal-general-en", "plan11-signal-semantics-en", "plan11-signal-untrusted-log", "plan11-signal-untrusted-label", "plan11-signal-availability-zh", "plan11-signal-availability-en", "plan11-signal-budget"}
	}
	if phase == "plan11-authoring-ui-replay" {
		return nil // Local scripted provider only; no new autonomous model task.
	}
	if phase == "plan11-authoring" {
		return []string{"plan11-authoring-zh", "plan11-authoring-en"}
	}
	if plan11NavigationPhase(phase) {
		names := []string{"plan11-dashboard-filters-zh", "plan11-dashboard-filters-en", "plan11-scope-visibility-restricted", "plan11-scope-visibility-reset"}
		if phase == "plan11-navigation-publish" {
			names = append(names, "plan11-authoring-zh", "plan11-authoring-en")
		}
		return names
	}
	if phase == "plan11-publish" {
		return append(plan8SelectedModelNames("plan11-workflows"), "plan11-authoring-zh", "plan11-authoring-en")
	}
	if phase == "plan11-workflows" {
		return []string{"plan11-dashboard-filters-zh", "plan11-dashboard-filters-en", "plan11-scope-general", "plan11-scope-restricted", "plan11-scope-reset"}
	}
	if phase == "plan11-metrics" {
		var names []string
		for _, locale := range []string{"zh", "en"} {
			for repeat := 1; repeat <= 3; repeat++ {
				names = append(names, fmt.Sprintf("plan11-metric-semantics-%s-%d", locale, repeat))
			}
		}
		return names
	}
	if phase == "plan11-metadata" {
		var names []string
		for _, locale := range []string{"zh", "en"} {
			for repeat := 1; repeat <= 3; repeat++ {
				names = append(names, fmt.Sprintf("plan11-sdk-coverage-%s-%d", locale, repeat), fmt.Sprintf("plan11-request-activity-%s-%d", locale, repeat))
			}
		}
		return names
	}
	if phase == "plan11-identity" {
		var names []string
		for _, locale := range []string{"zh", "en"} {
			for repeat := 1; repeat <= 3; repeat++ {
				names = append(names, fmt.Sprintf("plan11-placement-%s-%d", locale, repeat), fmt.Sprintf("plan11-identity-ask-%s-%d", locale, repeat))
			}
		}
		return names
	}
	if phase == "plan11-mcp" {
		var names []string
		for _, locale := range []string{"zh", "en"} {
			for repeat := 1; repeat <= 3; repeat++ {
				names = append(names, fmt.Sprintf("plan11-customer-mcp-%s-%d", locale, repeat))
			}
		}
		return names
	}
	if phase == "plan11-interactions" {
		return []string{"plan11-interaction-request-approval", "plan11-interaction-full-access", "plan11-stop-request-approval", "plan11-stop-full-access"}
	}
	if phase == "plan11-review" {
		return plan11ReviewModelNames(3, []string{"http"})
	}
	if phase == "plan11-targets" {
		return plan11TargetModelNames(nil)
	}
	if phase == "plan11-compare" {
		return plan11ComparisonModelNames(3)
	}
	result := []string{}
	for _, item := range []struct{ stage, name string }{
		{"kubernetes-namespace-workbench", "kubernetes_owned_workload_recovery"},
		{"service-instances-and-migration", "independent_collection_change"},
		{"sdk-application-protocols", "sdk_private_key_delivery"},
		{"real-model-applications-and-shared-database", "applications_apm_logs_shared_database"},
		{"real-model-secure-form", "secure_collection_form_and_followup"},
		{"real-model-collection-alert-health", "collection_alert_health"},
		{"real-model-application-revocation", "same_conversation_application_revocation"},
		{"real-model-workspace", "host_dashboard_workspace"},
	} {
		if item.name == "sdk_private_key_delivery" && !plan8SDKModelKeySelected(phase, "otlp_http") {
			continue
		}
		if plan8StageSelected(phase, item.stage) {
			result = append(result, item.name)
		}
	}
	if phase == "plan11" || phase == "plan11-discovery" || phase == "plan11-access" {
		if phase != "plan11-access" {
			result = append(result, "plan11-scope-general", "plan11-scope-restricted", "plan11-scope-reset")
		}
		for _, locale := range []string{"zh", "en"} {
			for repeat := 1; repeat <= 3; repeat++ {
				if phase != "plan11-access" {
					result = append(result, fmt.Sprintf("plan11-discovery-%s-%d", locale, repeat))
				}
				result = append(result, fmt.Sprintf("plan11-ambiguity-%s-%d", locale, repeat), fmt.Sprintf("plan11-injection-%s-%d", locale, repeat))
			}
		}
	}
	return result
}

// Data-only admission must not dispatch a paid model call. Protocol/transport
// selection is independent of how the test administrator obtains a scoped key.
func plan8SDKModelKeySelected(phase, protocol string) bool {
	return !plan11SignalPhase(phase) && phase != "integrations" && phase != "plan11-access" && phase != "plan11-review" && phase != "plan11-revocation" && phase != "plan11-mcp" && phase != "plan11-metadata" && phase != "plan11-metrics" && protocol == "otlp_http"
}

var plan8IntegrationGroups = []string{"mysql", "redis-nginx", "postgresql", "cache-web-coordination", "rabbitmq", "mongodb", "elasticsearch", "oracle", "sqlserver", "kafka", "http-tcp-tls-dns-icmp"}

var plan8IntegrationMembers = map[string][]string{
	"mysql": {"mysql"}, "redis-nginx": {"redis", "nginx"}, "postgresql": {"postgresql"},
	"cache-web-coordination": {"memcached", "apache", "haproxy", "zookeeper"},
	"rabbitmq":               {"rabbitmq"}, "mongodb": {"mongodb"}, "elasticsearch": {"elasticsearch"},
	"oracle": {"oracle"}, "sqlserver": {"sqlserver"}, "kafka": {"kafka"},
	"http-tcp-tls-dns-icmp": {"http", "tcp", "tls", "dns", "icmp"},
}

func plan11TargetModelNames(selected []string) []string {
	result := []string{}
	for _, group := range plan8IntegrationGroups {
		if plan8IntegrationGroupSelected(selected, group) {
			for _, engine := range plan8IntegrationMembers[group] {
				if !plan8IntegrationTargetSelected(selected, group, engine) {
					continue
				}
				result = append(result, "plan11-target-"+engine+"-zh-1")
			}
		}
	}
	return result
}

func plan8IntegrationGroupSelected(selected []string, group string) bool {
	if len(selected) == 0 {
		return true
	}
	for _, name := range selected {
		if name == group {
			return true
		}
		for _, engine := range plan8IntegrationMembers[group] {
			if name == engine {
				return true
			}
		}
	}
	return false
}
func plan8IntegrationTargetSelected(selected []string, group, engine string) bool {
	if len(selected) == 0 {
		return true
	}
	for _, name := range selected {
		if name == group || name == engine {
			return true
		}
	}
	return false
}

func plan8ParseIntegrationGroups(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	result := []string{}
	seen := map[string]bool{}
	for _, v := range strings.Split(value, ",") {
		known := false
		for _, name := range plan8IntegrationGroups {
			known = known || name == v
			for _, engine := range plan8IntegrationMembers[name] {
				known = known || engine == v
			}
		}
		if !known || seen[v] {
			return nil, errors.New("unknown or duplicate dedicated integration group")
		}
		seen[v] = true
		result = append(result, v)
	}
	return result, nil
}

func plan8StageSelected(phase, name string) bool {
	if name == "plan11-signal-cohort" || name == "plan11-namespace-recreation" {
		return plan11SignalPhase(phase)
	}
	if plan11SignalPhase(phase) {
		return name == "sdk-application-protocols" || name == "kubernetes-namespace-workbench" || name != "plan11-chat-interactions" && plan8StageSelected("plan11-interactions", name)
	}
	if name == "plan11-authoring-ui-replay" || name == "plan11-authoring-ui-production-readback" {
		return phase == "plan11-authoring-ui-replay"
	}
	if phase == "plan11-authoring-ui-replay" {
		return plan8StageSelected("plan11-interactions", name) && name != "plan11-chat-interactions"
	}
	if name == "plan11-navigation-ui" {
		return plan11NavigationPhase(phase)
	}
	if name == "plan11-dashboard-authoring" {
		return phase == "plan11-publish" || phase == "plan11-navigation-publish" || phase == "plan11-authoring"
	}
	if phase == "plan11-authoring" {
		return name != "plan11-business-workflows" && plan8StageSelected("plan11-workflows", name)
	}
	if plan11NavigationPhase(phase) {
		return name != "plan11-business-workflows" && plan8StageSelected("plan11-workflows", name)
	}
	if phase == "plan11-publish" {
		return plan8StageSelected("plan11-workflows", name)
	}
	if name == "plan11-business-workflows" {
		return phase == "plan11-workflows"
	}
	if phase == "plan11-workflows" {
		return name != "plan11-chat-interactions" && plan8StageSelected("plan11-interactions", name)
	}
	if name == "plan11-metric-semantics" {
		return phase == "plan11-metrics"
	}
	if phase == "plan11-metrics" {
		return name == "sdk-application-protocols" || name != "plan11-chat-interactions" && plan8StageSelected("plan11-interactions", name)
	}
	if name == "plan11-metadata-semantics" {
		return phase == "plan11-metadata"
	}
	if phase == "plan11-metadata" {
		return name == "sdk-application-protocols" || name != "plan11-chat-interactions" && plan8StageSelected("plan11-interactions", name)
	}
	if name == "plan11-resource-identity" {
		return phase == "plan11-identity"
	}
	if phase == "plan11-identity" {
		return name != "plan11-chat-interactions" && plan8StageSelected("plan11-interactions", name)
	}
	if name == "plan11-customer-mcp" {
		return phase == "plan11-mcp"
	}
	if phase == "plan11-mcp" {
		return name != "plan11-chat-interactions" && plan8StageSelected("plan11-interactions", name)
	}
	if name == "plan11-chat-interactions" {
		return phase == "plan11-interactions"
	}
	if phase == "plan11-interactions" {
		switch name {
		case "workspace-device-prerequisites", "workspace", "control-prerequisite", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api":
			return true
		default:
			return false
		}
	}
	if name == "plan11-read-only-dashboards" {
		return phase == "plan11-compare" || phase == "plan11-review" || phase == "plan11" || phase == "" || phase == "all"
	}
	if phase == "plan11-revocation" {
		return name == "sdk-application-protocols" || name == "real-model-application-revocation" || name != "plan11-profile-comparison" && plan8StageSelected("plan11-compare", name)
	}
	if phase == "plan11-review" {
		return name == "plan11-profile-comparison" || plan8StageSelected("plan11-access", name) || plan8StageSelected("plan11-targets", name)
	}
	if phase == "plan11-access" {
		if name == "sdk-application-protocols" || name == "real-model-application-revocation" || name == "plan11-resource-navigation" {
			return true
		}
		return name != "plan11-business-discovery" && plan8StageSelected("plan11-discovery", name)
	}
	if phase == "plan11-business" {
		return plan8StageSelected("models", name) || plan8StageSelected("kubernetes", name)
	}
	if name == "plan11-profile-comparison" {
		return phase == "plan11-compare"
	}
	if name == "plan11-business-discovery" {
		return phase == "plan11" || phase == "plan11-discovery"
	}
	if name == "plan11-resource-navigation" {
		return phase == "plan11" || phase == "plan11-discovery"
	}
	if phase == "plan11" {
		return true
	}
	if phase == "" || phase == "all" {
		return true
	}
	if phase != "integrations" && phase != "kubernetes" && phase != "workspace" && phase != "models" && phase != "plan11-discovery" && phase != "plan11-targets" && phase != "plan11-compare" {
		return false
	}
	switch name {
	case "workspace-device-prerequisites", "workspace", "control-prerequisite", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api":
		return true
	case "additional-supported-integrations":
		return phase == "integrations" || phase == "plan11-targets"
	case "kubernetes-namespace-workbench":
		return phase == "kubernetes"
	case "real-model-workspace":
		return phase == "workspace" || phase == "models"
	case "sdk-application-protocols":
		return phase == "models" || phase == "integrations"
	case "service-instances-and-migration",
		"real-model-applications-and-shared-database", "real-model-secure-form",
		"real-model-collection-alert-health", "real-model-application-revocation":
		return phase == "models"
	default:
		return false
	}
}

func plan11BusinessWorkflowPhase(phase string) bool {
	return phase == "plan11-workflows" || phase == "plan11-publish"
}

func plan11NavigationPhase(phase string) bool {
	return phase == "plan11-navigation-ui" || phase == "plan11-navigation-publish"
}
