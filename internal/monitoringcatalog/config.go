package monitoringcatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrConfig = errors.New("invalid monitoring configuration")
var ErrAdapter = errors.New("monitoring adapter unavailable")

// TargetIdentity is deliberately separate from sampling settings. It cannot
// contain credentials, arbitrary receiver configuration or ${...} expansion.
type TargetIdentity struct {
	Endpoint  string   `json:"endpoint"`
	Database  string   `json:"database,omitempty"`
	Members   []string `json:"members,omitempty"`
	Hostnames []string `json:"hostnames,omitempty"`
}

type TLS struct {
	Enabled    bool   `json:"enabled"`
	ServerName string `json:"server_name,omitempty"`
}

type Config struct {
	Interval             string   `json:"interval"`
	Timeout              string   `json:"timeout"`
	TLS                  TLS      `json:"tls"`
	AllowNativePasswords bool     `json:"allow_native_passwords,omitempty"`
	ReplicaSet           string   `json:"replica_set,omitempty"`
	AuthSource           string   `json:"auth_source,omitempty"`
	DirectConnection     bool     `json:"direct_connection,omitempty"`
	TopicMatch           string   `json:"topic_match,omitempty"`
	GroupMatch           string   `json:"group_match,omitempty"`
	SASLMechanism        string   `json:"sasl_mechanism,omitempty"`
	Nodes                []string `json:"nodes,omitempty"`
	Indices              []string `json:"indices,omitempty"`
	Method               string   `json:"method,omitempty"`
	DNSNetwork           string   `json:"dns_network,omitempty"`
	RecordType           string   `json:"record_type,omitempty"`
	PingCount            int      `json:"ping_count,omitempty"`
}

type Credentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func decodeStrict(raw []byte, out any) error {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return ErrConfig
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return ErrConfig
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return ErrConfig
	}
	return nil
}

func Parse(key string, identityJSON, configJSON []byte) (TargetIdentity, Config, error) {
	var identity TargetIdentity
	cfg := Config{Interval: "30s", Timeout: "10s", AllowNativePasswords: key == "mysql"}
	entry, exists := Lookup(key)
	if !exists || !entry.AdapterReady {
		return identity, cfg, ErrAdapter
	}
	if decodeStrict(identityJSON, &identity) != nil || decodeStrict(configJSON, &cfg) != nil {
		return identity, cfg, ErrConfig
	}
	if identity.Endpoint == "" || len(identity.Endpoint) > 2048 || strings.ContainsAny(identity.Endpoint, "$\x00\r\n") || strings.ContainsAny(identity.Database, "$\x00\r\n") || len(identity.Database) > 128 {
		return identity, cfg, ErrConfig
	}
	if err := validateNativeTarget(key, &identity, &cfg); err != nil {
		return identity, cfg, err
	}
	interval, e1 := time.ParseDuration(cfg.Interval)
	timeout, e2 := time.ParseDuration(cfg.Timeout)
	if e1 != nil || e2 != nil || interval < time.Second || interval > time.Hour || timeout < time.Second || timeout > interval || timeout > time.Minute {
		return identity, cfg, ErrConfig
	}
	if strings.ContainsAny(cfg.TLS.ServerName, "$\x00\r\n/: ") || len(cfg.TLS.ServerName) > 253 || (!cfg.TLS.Enabled && cfg.TLS.ServerName != "") || (key != "mysql" && cfg.AllowNativePasswords) {
		return identity, cfg, ErrConfig
	}
	cfg.Interval = interval.String()
	cfg.Timeout = timeout.String()
	return identity, cfg, nil
}

func ValidateCredentials(key string, raw []byte) error {
	var value Credentials
	if len(raw) == 0 || len(raw) > 64<<10 || decodeStrict(raw, &value) != nil || strings.ContainsAny(value.Username, "\x00\r\n") || len(value.Username) > 256 || strings.ContainsRune(value.Password, '\x00') {
		return ErrConfig
	}
	return validateNativeCredentials(key, value)
}

// ReceiverConfig emits only controlled fields and empty authentication slots.
// The executor installs an authenticated private overlay separately. Secrets
// never appear in a bundle, action plan, command argument or diagnostic.
func ReceiverConfig(key string, collection uuid.UUID, identityJSON, configJSON []byte, hasCredential bool) (map[string]any, error) {
	identity, cfg, err := Parse(key, identityJSON, configJSON)
	if err != nil {
		return nil, err
	}
	if collection == uuid.Nil || (credentialsRequired(key) && !hasCredential) || (!credentialsSupported(key) && hasCredential) {
		return nil, ErrConfig
	}
	return nativeReceiver(key, identity, cfg), nil
}
