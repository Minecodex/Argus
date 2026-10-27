package argusdev

import (
	"errors"
	"strings"
)

var plan8IntegrationGroups = []string{"postgresql", "cache-web-coordination", "rabbitmq", "mongodb", "elasticsearch", "oracle", "kafka", "http-tcp-tls-dns-icmp"}

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
	if phase == "" || phase == "all" {
		return true
	}
	if phase != "integrations" && phase != "kubernetes" && phase != "workspace" && phase != "models" {
		return false
	}
	switch name {
	case "workspace-device-prerequisites", "workspace", "control-prerequisite", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api":
		return true
	case "additional-supported-integrations":
		return phase == "integrations"
	case "kubernetes-namespace-workbench":
		return phase == "kubernetes"
	case "real-model-workspace":
		return phase == "workspace" || phase == "models"
	case "service-instances-and-migration", "sdk-application-protocols",
		"real-model-applications-and-shared-database", "real-model-secure-form",
		"real-model-collection-alert-health", "real-model-application-revocation":
		return phase == "models"
	default:
		return false
	}
}
