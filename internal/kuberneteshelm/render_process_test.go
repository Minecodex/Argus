package kuberneteshelm

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"helm.sh/helm/v4/pkg/chart/common"
)

func TestMain(m *testing.M) {
	if RunRenderCommand(os.Args[1:], os.Stdin, os.Stdout) {
		return
	}
	os.Exit(m.Run())
}
func TestHelmRendererIsolatesValuesLogsAndRejectsExternalSchema(t *testing.T) {
	r := Request{Name: "sample", Namespace: Namespace{Name: "orders"}, Operation: "install"}
	template := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: sample\ndata:\n  message: {{ .Values.message | quote }}\n"
	files := map[string]string{"sample/Chart.yaml": "apiVersion: v2\nname: sample\nversion: 1.0.0\n", "sample/templates/objects.yaml": template, "sample/values.yaml": "message: default\n"}
	material := Material{Chart: packagedFiles(t, files), Values: []byte("message: protected-render-canary\n")}
	result, e := renderIsolated(t.Context(), r, material, common.DefaultCapabilities.Copy())
	if e != nil || !bytes.Contains([]byte(result.Manifest), []byte("protected-render-canary")) {
		t.Fatal("bounded renderer", e)
	}
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); _, _ = w.Write([]byte(`{"type":"object"}`)) }))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "private-schema.json")
	if e = os.WriteFile(file, []byte(`{"type":"object"}`), 0600); e != nil {
		t.Fatal(e)
	}
	path := filepath.ToSlash(file)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	fileURL := (&url.URL{Scheme: "file", Path: path}).String()
	for _, reference := range []string{server.URL + "/schema.json", fileURL} {
		files["sample/values.schema.json"] = `{"$ref":"` + reference + `"}`
		material.Chart = packagedFiles(t, files)
		if _, e = renderIsolated(t.Context(), r, material, common.DefaultCapabilities.Copy()); e == nil {
			t.Fatal("external schema was allowed", reference)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("renderer made an outbound schema request")
	}
	files["sample/values.schema.json"] = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"message":{"$ref":"#/$defs/message"}},"$defs":{"message":{"type":"string"}}}`
	material.Chart = packagedFiles(t, files)
	if _, e = renderIsolated(t.Context(), r, material, common.DefaultCapabilities.Copy()); e != nil {
		t.Fatal("embedded schema failed", e)
	}
	material.Values = []byte("message: 123\n")
	if _, e = renderIsolated(t.Context(), r, material, common.DefaultCapabilities.Copy()); e == nil {
		t.Fatal("values schema bypassed")
	}
}
func TestHelmRendererRejectsRunawayTemplateAndHonorsCancellation(t *testing.T) {
	r := Request{Name: "sample", Namespace: Namespace{Name: "orders"}, Operation: "install"}
	material := Material{Chart: packagedChart(t, "1.0.0", `{{ "x" | repeat 1073741824 }}`)}
	start := time.Now()
	if _, e := renderIsolated(t.Context(), r, material, common.DefaultCapabilities.Copy()); e == nil {
		t.Fatal("unbounded allocation accepted")
	}
	if time.Since(start) > 12*time.Second {
		t.Fatal("renderer outlived wall clock limit")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e := renderIsolated(ctx, r, material, common.DefaultCapabilities.Copy()); e == nil {
		t.Fatal("cancelled renderer started")
	}
}
