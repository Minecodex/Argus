package kubernetespodlogs

import (
	"errors"
	"testing"

	"github.com/kakj-go/Argus/internal/kubernetesaccess"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestLogCredentialPinsServerAndRejectsLocalExecutionAndFiles(t *testing.T) {
	base := clientcmdapi.Config{CurrentContext: "test", Clusters: map[string]*clientcmdapi.Cluster{"test": {Server: "https://unselected.example.test"}}, AuthInfos: map[string]*clientcmdapi.AuthInfo{"test": {Token: "controlled-token"}}, Contexts: map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test"}}}
	for _, kind := range []string{"inline", "ca-file", "certificate-file", "key-file", "token-file", "exec", "provider", "proxy", "impersonation-user", "impersonation-uid", "impersonation-group", "impersonation-extra", "skip-tls"} {
		t.Run(kind, func(t *testing.T) {
			cfg := base.DeepCopy()
			cluster, auth := cfg.Clusters["test"], cfg.AuthInfos["test"]
			switch kind {
			case "ca-file":
				cluster.CertificateAuthority = "/must-not-read/ca.pem"
			case "certificate-file":
				auth.ClientCertificate = "/must-not-read/cert.pem"
			case "key-file":
				auth.ClientKey = "/must-not-read/key.pem"
			case "token-file":
				auth.TokenFile = "/must-not-read/token"
			case "exec":
				auth.Exec = &clientcmdapi.ExecConfig{Command: "must-not-execute"}
			case "provider":
				auth.AuthProvider = &clientcmdapi.AuthProviderConfig{Name: "must-not-load"}
			case "proxy":
				cluster.ProxyURL = "http://must-not-connect.example.test"
			case "impersonation-user":
				auth.Impersonate = "another-user"
			case "impersonation-uid":
				auth.ImpersonateUID = "another-uid"
			case "impersonation-group":
				auth.ImpersonateGroups = []string{"system:masters"}
			case "impersonation-extra":
				auth.ImpersonateUserExtra = map[string][]string{"scope": {"admin"}}
			case "skip-tls":
				cluster.InsecureSkipTLSVerify = true
			}
			raw, e := clientcmd.Write(*cfg)
			if e != nil {
				t.Fatal(e)
			}
			result, e := kubernetesaccess.Config(raw, "https://selected.example.test")
			if kind == "inline" {
				if e != nil || result.Host != "https://selected.example.test" || result.BearerToken != "controlled-token" {
					t.Fatal("inline identity or canonical server changed", e)
				}
			} else if !errors.Is(e, kubernetesaccess.ErrUnsafeCredential) || result != nil {
				t.Fatal("external authentication input was accepted", e)
			}
		})
	}
	raw, _ := clientcmd.Write(base)
	for _, bad := range []string{"http://selected.example.test", "https://user@selected.example.test", "https://selected.example.test?target=other", "https://selected.example.test#other"} {
		if _, e := kubernetesaccess.Config(raw, bad); !errors.Is(e, kubernetesaccess.ErrUnsafeCredential) {
			t.Fatal("unsafe destination accepted")
		}
	}
}
