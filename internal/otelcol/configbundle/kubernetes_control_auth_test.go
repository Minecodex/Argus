package configbundle

import (
	"github.com/kakj-go/Argus/internal/builtincollection"
	"testing"
)

func TestControlMetricsKeepTargetTrustSeparateFromDiscovery(t *testing.T) {
	for _, key := range []string{"kubernetes_scheduler_metrics", "kubernetes_controller_metrics"} {
		value := builtincollection.Config{Interval: "5s", ComponentTLS: &builtincollection.ComponentTLS{CAPEM: "PUBLIC_COMPONENT_CA", ServerName: "component.internal"}}
		receiver := kubernetesControlMetricReceiver(key, value)
		job := receiver["config"].(map[string]any)["scrape_configs"].([]map[string]any)[0]
		if job["metrics_path"] != "/metrics" || job["scheme"] != "https" {
			t.Fatal("secure metrics routed through API proxy")
		}
		tls := job["tls_config"].(map[string]any)
		if tls["insecure_skip_verify"] != false || tls["ca"] != "PUBLIC_COMPONENT_CA" || tls["ca_file"] != nil || tls["server_name"] != "component.internal" {
			t.Fatalf("target trust: %+v", tls)
		}
		discovery := job["kubernetes_sd_configs"].([]map[string]any)[0]
		discoveryTLS := discovery["tls_config"].(map[string]any)
		if discoveryTLS["ca"] != nil || discoveryTLS["ca_file"] != "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt" || discoveryTLS["insecure_skip_verify"] != false {
			t.Fatal("component CA altered API discovery trust")
		}
		for _, client := range []map[string]any{job, discovery} {
			if client["authorization"].(map[string]any)["credentials_file"] != "/var/run/secrets/kubernetes.io/serviceaccount/token" {
				t.Fatal("caller controlled the runtime token")
			}
		}
		ipv6 := false
		for _, rule := range job["relabel_configs"].([]map[string]any) {
			if rule["replacement"] == "kubernetes.default.svc:443" {
				t.Fatal("secure target is API server")
			}
			if rule["regex"] == "(.+:.*)" {
				ipv6 = true
			}
		}
		if !ipv6 {
			t.Fatal("IPv6 Pod targets lack bracketed address")
		}
	}
	etcd := kubernetesControlMetricReceiver("kubernetes_etcd_metrics", builtincollection.Config{Interval: "30s"})
	job := etcd["config"].(map[string]any)["scrape_configs"].([]map[string]any)[0]
	proxy := false
	for _, rule := range job["relabel_configs"].([]map[string]any) {
		if rule["replacement"] == "kubernetes.default.svc:443" {
			proxy = true
		}
	}
	if !proxy {
		t.Fatal("native HTTP etcd metrics bypassed authenticated API")
	}
}
