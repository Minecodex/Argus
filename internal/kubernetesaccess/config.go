// Package kubernetesaccess validates customer Kubernetes identities before
// client-go can load local files or construct an authentication transport.
package kubernetesaccess

import (
	"errors"
	"net/url"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var ErrUnsafeCredential = errors.New("KUBERNETES_IDENTITY_UNAVAILABLE")

func ValidServer(server string) bool {
	u, err := url.Parse(server)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.Hostname() != "" && u.User == nil && u.Opaque == "" && u.RawQuery == "" && u.Fragment == ""
}

func Config(value []byte, server string) (*rest.Config, error) {
	var config *rest.Config
	var err error
	if len(value) == 0 {
		// Only the system-created in-cluster identity may reference its fixed files.
		config, err = rest.InClusterConfig()
	} else {
		if len(value) > 1<<20 {
			return nil, ErrUnsafeCredential
		}
		// Load parses metadata without resolving certificate/key/token references.
		parsed, e := clientcmd.Load(value)
		if e != nil {
			return nil, ErrUnsafeCredential
		}
		for _, cluster := range parsed.Clusters {
			if cluster == nil || cluster.CertificateAuthority != "" || cluster.InsecureSkipTLSVerify || cluster.ProxyURL != "" {
				return nil, ErrUnsafeCredential
			}
		}
		for _, identity := range parsed.AuthInfos {
			if identity == nil || identity.ClientCertificate != "" || identity.ClientKey != "" || identity.TokenFile != "" || identity.Exec != nil || identity.AuthProvider != nil || identity.Impersonate != "" || identity.ImpersonateUID != "" || len(identity.ImpersonateGroups) > 0 || len(identity.ImpersonateUserExtra) > 0 {
				return nil, ErrUnsafeCredential
			}
		}
		config, err = clientcmd.RESTConfigFromKubeConfig(value)
	}
	if err != nil {
		return nil, ErrUnsafeCredential
	}
	if server != "" {
		config.Host = server
	}
	if !ValidServer(config.Host) {
		return nil, ErrUnsafeCredential
	}
	return config, nil
}
