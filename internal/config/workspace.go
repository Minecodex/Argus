package config

import (
	"os"
	"strconv"
	"time"
)

type Workspace struct {
	ProcessLimit                                          int64
	ExecdImage                                            string
	EgressImage                                           string
	AdmissionCert, AdmissionKey                           string
	Enabled                                               bool
	Namespace                                             string
	StorageClass                                          string
	IOImage                                               string
	TLSSecret                                             string
	ClientCert, ClientKey, CAPath, ServerName, ClientName string
	DefaultBytes, EnterpriseBytes, MaxFileBytes           int64
	IdleTTL                                               time.Duration
	FilesBucket                                           string
}

func LoadWorkspace() Workspace {
	enabled, _ := strconv.ParseBool(os.Getenv("ARGUS_WORKSPACE_ENABLED"))
	positive := func(key string, fallback int64) int64 {
		value, err := strconv.ParseInt(os.Getenv(key), 10, 64)
		if err != nil || value <= 0 {
			return fallback
		}
		return value
	}
	namespace := valueOrDefault("ARGUS_WORKSPACE_NAMESPACE", "argus-workspaces")
	return Workspace{Enabled: enabled, Namespace: namespace, StorageClass: valueOrDefault("ARGUS_WORKSPACE_STORAGE_CLASS", "argus-workspace"), IOImage: os.Getenv("ARGUS_WORKSPACE_IO_IMAGE"),
		ProcessLimit: positive("ARGUS_WORKSPACE_PROCESS_LIMIT", 256),
		ExecdImage:   valueOrDefault("ARGUS_WORKSPACE_EXECD_IMAGE", "opensandbox/execd:v1.0.22@sha256:0d8f44cf4194732719aa79999d4b120c98bdab02bc61e9ad13f75f83af4c2684"),
		EgressImage:  os.Getenv("ARGUS_WORKSPACE_EGRESS_IMAGE"), AdmissionCert: os.Getenv("ARGUS_WORKSPACE_ADMISSION_CERT"), AdmissionKey: os.Getenv("ARGUS_WORKSPACE_ADMISSION_KEY"),
		TLSSecret: valueOrDefault("ARGUS_WORKSPACE_TLS_SECRET", "argus-workspace-io-tls"), ClientCert: os.Getenv("ARGUS_WORKSPACE_CLIENT_CERT"), ClientKey: os.Getenv("ARGUS_WORKSPACE_CLIENT_KEY"),
		CAPath: os.Getenv("ARGUS_WORKSPACE_CA"), ServerName: valueOrDefault("ARGUS_WORKSPACE_SERVER_NAME", "workspace-io."+namespace+".svc"), ClientName: valueOrDefault("ARGUS_WORKSPACE_CLIENT_NAME", "argus-workspace-client"),
		DefaultBytes: positive("ARGUS_WORKSPACE_BYTES", 2<<30), EnterpriseBytes: positive("ARGUS_WORKSPACE_ENTERPRISE_BYTES", 20<<30), MaxFileBytes: positive("ARGUS_WORKSPACE_MAX_FILE_BYTES", 100<<20),
		IdleTTL: time.Duration(positive("ARGUS_WORKSPACE_IDLE_SECONDS", 900)) * time.Second, FilesBucket: valueOrDefault("ARGUS_WORKSPACE_FILES_BUCKET", "argus-workspace-files")}
}
