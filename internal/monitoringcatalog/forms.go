package monitoringcatalog

import "slices"

// Form describes public product fields, never arbitrary Collector configuration.
// The same registry drives the API and the generated portal catalog.
type Form struct {
	EndpointKind     string `json:"endpoint_kind"`
	Authentication   string `json:"authentication"`
	Database         bool   `json:"database"`
	DatabaseRequired bool   `json:"database_required"`
	Members          bool   `json:"members"`
	Hostnames        bool   `json:"hostnames"`
	TLS              bool   `json:"tls"`
	TLSServerName    bool   `json:"tls_server_name"`
}

func nativeForm(key string) Form {
	f := Form{EndpointKind: "host_port", Authentication: "none"}
	if credentialsRequired(key) {
		f.Authentication = "required"
	} else if credentialsSupported(key) {
		f.Authentication = "optional"
	}
	f.Database = slices.Contains([]string{"mysql", "postgresql", "sqlserver", "oracle"}, key)
	f.DatabaseRequired = key == "oracle"
	f.Members = key == "mongodb" || key == "kafka"
	f.Hostnames = key == "dns"
	f.TLS = slices.Contains([]string{"mysql", "postgresql", "sqlserver", "mongodb", "redis", "memcached", "kafka"}, key)
	f.TLSServerName = f.TLS && key != "postgresql" && key != "sqlserver"
	if httpIntegration(key) {
		f.EndpointKind = "http_url"
		f.TLSServerName = true
	}
	if key == "icmp" {
		f.EndpointKind = "host"
	}
	if key == "iis" {
		f.EndpointKind = "local"
	}
	return f
}
