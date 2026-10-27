package monitoringcatalog

import (
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

func httpIntegration(key string) bool {
	return slices.Contains([]string{"nginx", "apache", "haproxy", "rabbitmq", "elasticsearch", "http"}, key)
}
func hostPort(value string) bool {
	host, port, err := net.SplitHostPort(value)
	p, e := strconv.Atoi(port)
	return err == nil && e == nil && p > 0 && p <= 65535 && host != "" && !strings.ContainsAny(host, "$\x00\r\n/\\ @")
}
func plainName(value string) bool {
	return value != "" && len(value) <= 253 && !strings.ContainsAny(value, "$\x00\r\n/\\ @:")
}
func credentialsRequired(key string) bool {
	return slices.Contains([]string{"mysql", "postgresql", "sqlserver", "oracle", "rabbitmq"}, key)
}
func credentialsSupported(key string) bool {
	return credentialsRequired(key) || slices.Contains([]string{"redis", "mongodb", "kafka", "elasticsearch", "nginx", "apache", "haproxy", "http"}, key)
}

func validateNativeCredentials(key string, value Credentials) error {
	if !credentialsSupported(key) {
		return ErrAdapter
	}
	if httpIntegration(key) && strings.Contains(value.Username, ":") {
		return ErrConfig
	}
	if key == "redis" {
		return nil
	}
	if value.Username == "" {
		return ErrConfig
	}
	if key != "mysql" && value.Password == "" {
		return ErrConfig
	}
	return nil
}

func validateNativeTarget(key string, identity *TargetIdentity, cfg *Config) error {
	if httpIntegration(key) {
		u, err := url.Parse(identity.Endpoint)
		if err != nil || !slices.Contains([]string{"http", "https"}, u.Scheme) || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return ErrConfig
		}
		if u.RawQuery != "" && !(key == "apache" && u.RawQuery == "auto") {
			return ErrConfig
		}
		if key == "apache" && u.RawQuery != "auto" {
			return ErrConfig
		}
		cfg.TLS.Enabled = u.Scheme == "https"
	} else if key == "icmp" {
		if !plainName(identity.Endpoint) && net.ParseIP(identity.Endpoint) == nil {
			return ErrConfig
		}
	} else if key == "iis" {
		if identity.Endpoint != "local" {
			return ErrConfig
		}
	} else if !hostPort(identity.Endpoint) {
		return ErrConfig
	}
	if identity.Database != "" && !slices.Contains([]string{"mysql", "postgresql", "oracle", "sqlserver"}, key) {
		return ErrConfig
	}
	if key == "oracle" && identity.Database == "" {
		return ErrConfig
	}
	if len(identity.Members) > 16 || len(identity.Members) > 0 && !slices.Contains([]string{"mongodb", "kafka"}, key) {
		return ErrConfig
	}
	for _, member := range identity.Members {
		if !hostPort(member) {
			return ErrConfig
		}
	}
	if key == "dns" {
		if len(identity.Hostnames) == 0 || len(identity.Hostnames) > 32 {
			return ErrConfig
		}
		for _, host := range identity.Hostnames {
			if !plainName(host) {
				return ErrConfig
			}
		}
	} else if len(identity.Hostnames) > 0 {
		return ErrConfig
	}
	if cfg.TLS.Enabled && slices.Contains([]string{"oracle", "zookeeper", "iis", "tcp", "icmp", "dns", "tls"}, key) {
		return ErrConfig
	}
	if cfg.TLS.ServerName != "" && slices.Contains([]string{"postgresql", "oracle", "sqlserver", "tls"}, key) {
		return ErrConfig
	}
	if key != "mongodb" && (cfg.ReplicaSet != "" || cfg.AuthSource != "" || cfg.DirectConnection) {
		return ErrConfig
	}
	for _, value := range []string{cfg.ReplicaSet, cfg.AuthSource} {
		if value != "" && !plainName(value) {
			return ErrConfig
		}
	}
	if key != "kafka" && (cfg.TopicMatch != "" || cfg.GroupMatch != "" || cfg.SASLMechanism != "") {
		return ErrConfig
	}
	for _, pattern := range []string{cfg.TopicMatch, cfg.GroupMatch} {
		if len(pattern) > 256 {
			return ErrConfig
		}
		if pattern != "" {
			if _, err := regexp.Compile(pattern); err != nil {
				return ErrConfig
			}
		}
	}
	if cfg.SASLMechanism != "" && !slices.Contains([]string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"}, cfg.SASLMechanism) {
		return ErrConfig
	}
	if key != "elasticsearch" && (len(cfg.Nodes) > 0 || len(cfg.Indices) > 0) {
		return ErrConfig
	}
	for _, values := range [][]string{cfg.Nodes, cfg.Indices} {
		if len(values) > 100 {
			return ErrConfig
		}
		for _, value := range values {
			if value == "" || len(value) > 256 || strings.ContainsAny(value, "$\x00\r\n") {
				return ErrConfig
			}
		}
	}
	if cfg.Method != "" && (key != "http" || !slices.Contains([]string{"GET", "HEAD", "OPTIONS"}, cfg.Method)) {
		return ErrConfig
	}
	if key != "dns" && (cfg.DNSNetwork != "" || cfg.RecordType != "") {
		return ErrConfig
	}
	if cfg.DNSNetwork != "" && !slices.Contains([]string{"udp", "tcp", "tcp-tls"}, cfg.DNSNetwork) {
		return ErrConfig
	}
	if cfg.RecordType != "" && !slices.Contains([]string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SOA", "SRV"}, cfg.RecordType) {
		return ErrConfig
	}
	if cfg.PingCount < 0 || cfg.PingCount > 10 || key != "icmp" && cfg.PingCount != 0 {
		return ErrConfig
	}
	return nil
}
