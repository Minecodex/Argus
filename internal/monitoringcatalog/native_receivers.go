package monitoringcatalog

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

func nativeTLS(cfg Config) map[string]any {
	result := map[string]any{"insecure": !cfg.TLS.Enabled}
	if cfg.TLS.ServerName != "" {
		result["server_name_override"] = cfg.TLS.ServerName
	}
	return result
}

func nativeReceiver(key string, identity TargetIdentity, cfg Config) map[string]any {
	result := map[string]any{"collection_interval": cfg.Interval, "timeout": cfg.Timeout}
	switch key {
	case "mysql", "redis", "postgresql", "memcached":
		result["endpoint"], result["transport"], result["tls"] = identity.Endpoint, "tcp", nativeTLS(cfg)
		if key == "mysql" {
			result["database"], result["allow_native_passwords"] = identity.Database, cfg.AllowNativePasswords
		}
		if key == "postgresql" && identity.Database != "" {
			result["databases"] = []string{identity.Database}
		}
		if key == "postgresql" {
			// Upstream emits these aggregate counts even when authentication or
			// discovery fails. Zero is not evidence that the database was read.
			result["metrics"] = map[string]any{"postgresql.database.count": map[string]any{"enabled": false}, "postgresql.table.count": map[string]any{"enabled": false}}
		}
	case "sqlserver":
		result["datasource"] = sqlServerDSN(identity, cfg, Credentials{})
	case "oracle":
		result["endpoint"], result["service"] = identity.Endpoint, identity.Database
	case "mongodb":
		hosts := []map[string]string{}
		for _, endpoint := range append([]string{identity.Endpoint}, identity.Members...) {
			hosts = append(hosts, map[string]string{"endpoint": endpoint})
		}
		result["hosts"], result["tls"], result["direct_connection"] = hosts, nativeTLS(cfg), cfg.DirectConnection
		if cfg.ReplicaSet != "" {
			result["replica_set"] = cfg.ReplicaSet
		}
		if cfg.AuthSource != "" {
			result["auth_source"] = cfg.AuthSource
		}
	case "kafka":
		result["brokers"], result["tls"] = append([]string{identity.Endpoint}, identity.Members...), nativeTLS(cfg)
		result["scrapers"] = []string{"brokers", "topics", "consumers"}
		if cfg.TopicMatch != "" {
			result["topic_match"] = strings.ReplaceAll(cfg.TopicMatch, "$", "$$")
		}
		if cfg.GroupMatch != "" {
			result["group_match"] = strings.ReplaceAll(cfg.GroupMatch, "$", "$$")
		}
	case "nginx", "apache", "haproxy", "rabbitmq", "elasticsearch":
		result["endpoint"], result["tls"] = identity.Endpoint, nativeTLS(cfg)
		if key == "rabbitmq" {
			result["metrics"] = map[string]any{"rabbitmq.node.mem_used": map[string]any{"enabled": true}, "rabbitmq.node.disk_free": map[string]any{"enabled": true}, "rabbitmq.node.fd_used": map[string]any{"enabled": true}, "rabbitmq.node.uptime": map[string]any{"enabled": true}}
		}
		if key == "elasticsearch" {
			result["nodes"] = []string{"_all"}
			result["indices"] = []string{"_all"}
			if len(cfg.Nodes) > 0 {
				result["nodes"] = cfg.Nodes
			}
			if len(cfg.Indices) > 0 {
				result["indices"] = cfg.Indices
			}
		}
	case "zookeeper":
		result["endpoint"] = identity.Endpoint
	case "http":
		method := cfg.Method
		if method == "" {
			method = "GET"
		}
		result["targets"] = []map[string]any{{"endpoint": identity.Endpoint, "method": method, "timeout": cfg.Timeout, "tls": nativeTLS(cfg)}}
	case "tcp", "tls":
		result["targets"] = []map[string]string{{"endpoint": identity.Endpoint}}
	case "dns":
		network, record := cfg.DNSNetwork, cfg.RecordType
		if network == "" {
			network = "udp"
		}
		if record == "" {
			record = "A"
		}
		result["dns_servers"] = []map[string]any{{"endpoint": identity.Endpoint, "network": network, "timeout": cfg.Timeout}}
		hosts := []map[string]string{}
		for _, name := range identity.Hostnames {
			hosts = append(hosts, map[string]string{"name": name, "record_type": record})
		}
		result["hostnames"] = hosts
	case "icmp":
		count := cfg.PingCount
		if count == 0 {
			count = 3
		}
		result["targets"] = []map[string]any{{"host": identity.Endpoint, "ping_count": count, "ping_timeout": cfg.Timeout}}
	case "iis":
	}
	return result
}

func sqlServerDSN(identity TargetIdentity, cfg Config, credentials Credentials) string {
	u := url.URL{Scheme: "sqlserver", Host: identity.Endpoint}
	if credentials.Username != "" {
		u.User = url.UserPassword(credentials.Username, credentials.Password)
	}
	query := url.Values{}
	// The driver's "false" mode still negotiates TLS for the login packet.
	// Match the user's explicit disabled-TLS setting without a hidden
	// certificate-verification bypass. Enabled TLS remains strict and never
	// falls back to this mode after a handshake failure.
	query.Set("encrypt", "disable")
	if cfg.TLS.Enabled {
		query.Set("encrypt", "true")
		query.Set("TrustServerCertificate", "false")
	}
	if identity.Database != "" {
		query.Set("database", identity.Database)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

// CredentialOverlay compiles private authentication into the native receiver's
// fields. Targets and all non-authentication settings still come from the frozen
// public bundle. Dollar escaping happens only at this last confmap boundary.
func CredentialOverlay(key string, identityRaw, configRaw, credentialsRaw []byte) (map[string]any, error) {
	identity, cfg, err := Parse(key, identityRaw, configRaw)
	if err != nil {
		return nil, err
	}
	if err = ValidateCredentials(key, credentialsRaw); err != nil {
		return nil, err
	}
	var value Credentials
	if json.Unmarshal(credentialsRaw, &value) != nil {
		return nil, ErrConfig
	}
	escape := func(v string) string { return strings.ReplaceAll(v, "$", "$$") }
	if key == "sqlserver" {
		return map[string]any{"datasource": escape(sqlServerDSN(identity, cfg, value))}, nil
	}
	if key == "kafka" {
		mechanism := cfg.SASLMechanism
		if mechanism == "" {
			mechanism = "SCRAM-SHA-512"
		}
		return map[string]any{"auth": map[string]any{"sasl": map[string]string{"username": escape(value.Username), "password": escape(value.Password), "mechanism": mechanism}}}, nil
	}
	if key == "nginx" || key == "apache" || key == "haproxy" || key == "http" {
		header := "Basic " + base64.StdEncoding.EncodeToString([]byte(value.Username+":"+value.Password))
		if key == "http" {
			native := nativeReceiver(key, identity, cfg)
			targets := native["targets"].([]map[string]any)
			targets[0]["headers"] = map[string]string{"Authorization": header}
			return map[string]any{"targets": targets}, nil
		}
		return map[string]any{"headers": map[string]string{"Authorization": header}}, nil
	}
	return map[string]any{"username": escape(value.Username), "password": escape(value.Password)}, nil
}
