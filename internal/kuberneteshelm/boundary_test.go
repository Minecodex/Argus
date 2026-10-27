package kuberneteshelm

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestHelmMetadataAndStorageDoNotEscapeNamespaceOrDecodeBudgets(t *testing.T) {
	r := Request{Namespace: Namespace{Name: "orders"}, Operation: "list"}
	row := Release{Name: "sample", Namespace: "orders", Revision: 1, Status: "deployed", Chart: "sample", ChartVersion: "1.0.0", UpdatedAt: time.Now()}
	if e := ValidateReleases([]Release{row}, r); e != nil {
		t.Fatal(e)
	}
	other := row
	other.Namespace = "foreign"
	if e := ValidateReleases([]Release{other}, r); e == nil {
		t.Fatal("Connector moved release scope")
	}
	if e := ValidateReleases([]Release{row, row}, r); e == nil {
		t.Fatal("duplicate release metadata accepted")
	}
	var compressed bytes.Buffer
	zip := gzip.NewWriter(&compressed)
	_, _ = io.Copy(zip, strings.NewReader(strings.Repeat("x", (16<<20)+1)))
	_ = zip.Close()
	secret := corev1.Secret{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, Type: "helm.sh/release.v1", Data: map[string][]byte{"release": []byte(base64.StdEncoding.EncodeToString(compressed.Bytes()))}}
	raw, _ := json.Marshal(secret)
	if e := validateStorageResponse(raw); e == nil {
		t.Fatal("compressed release exceeded decode budget")
	}
	secret.Data["release"] = []byte(base64.StdEncoding.EncodeToString([]byte(`{"name":"safe"}`)))
	raw, _ = json.Marshal(secret)
	if e := validateStorageResponse(raw); e != nil {
		t.Fatal("legacy uncompressed Helm JSON rejected", e)
	}
}
func TestHelmStreamingBodyBudgetAndManagedFieldsComparison(t *testing.T) {
	body := &budgetBody{ReadCloser: io.NopCloser(strings.NewReader("streaming-output")), remaining: 5}
	buf := make([]byte, 2)
	if n, e := body.Read(buf); e != nil || n != 2 || string(buf) != "st" {
		t.Fatal("streaming response was buffered", e)
	}
	if _, e := io.ReadAll(body); e == nil {
		t.Fatal("stream exceeded budget")
	}
	base := map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "settings", "resourceVersion": "1"}, "data": map[string]any{"value": "protected"}}
	var a, b unstructured.Unstructured
	a.Object = base
	b = *a.DeepCopy()
	b.SetResourceVersion("2")
	b.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "helm", Operation: metav1.ManagedFieldsOperationApply}})
	if !sameManagedFieldsObject(&a, &b) {
		t.Fatal("metadata-only migration rejected")
	}
	b.Object["data"] = map[string]any{"value": "different"}
	if sameManagedFieldsObject(&a, &b) {
		t.Fatal("metadata migration changed business content")
	}
}
