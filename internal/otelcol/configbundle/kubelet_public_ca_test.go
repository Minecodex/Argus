package configbundle

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"github.com/google/uuid"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestNodeBuiltinUsesExplicitPublicCAWithoutHostCertificateOrTLSBypass(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Node public CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	cluster, id := uuid.NewString(), uuid.NewString()
	in := RenderInput{CollectorID: uuid.NewString(), ResourceID: cluster, ResourceType: "kubernetes_cluster", Role: "direct", Platform: "linux_amd64", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test"}
	for _, custom := range []bool{false, true} {
		config := json.RawMessage(`{"interval":"10s"}`)
		if custom {
			config, _ = json.Marshal(map[string]any{"interval": "10s", "component_tls": map[string]string{"ca_pem": ca}})
		}
		in.Collections = []Collection{{ID: id, Integration: "kubernetes_node_metrics", TargetType: "kubernetes_cluster", TargetID: cluster, Config: config, Revision: 1, AssignmentEpoch: 1, Enabled: true}}
		raw, e := Render(in)
		if e != nil {
			t.Fatal(e)
		}
		files, err := KubernetesPublicFiles(raw)
		if err != nil || custom && files[KubeletCAFile] != ca || !custom && len(files) != 0 {
			t.Fatalf("unexpected public CA files: %v %v", files, err)
		}
		agent := decodeTarget(t, raw, "kubernetes_agent")
		receiver := nestedMap(t, agent, "receivers", "kubeletstats/collection_"+strings.ReplaceAll(id, "-", ""))
		if receiver["insecure_skip_verify"] != false || receiver["auth_type"] != "serviceAccount" || receiver["endpoint"] != "https://${env:K8S_NODE_NAME}:10250" {
			t.Fatal("Node auth or destination weakened", receiver)
		}
		if custom {
			if receiver["ca_file"] != KubeletCAPath || receiver["ca_pem"] != nil || receiver["server_name_override"] != nil {
				t.Fatal("Explicit node trust ignored", receiver)
			}
		} else if receiver["ca_file"] != "/var/run/argus-kubelet/pki/kubelet.crt" || receiver["ca_pem"] != nil {
			t.Fatal("Legacy self-signed node default changed", receiver)
		}
	}
}
