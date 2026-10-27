package kuberneteshelm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"testing"
)

func packagedChart(t *testing.T, version, template string) []byte {
	t.Helper()
	return packagedFiles(t, map[string]string{"sample/Chart.yaml": "apiVersion: v2\nname: sample\nversion: " + version + "\n", "sample/templates/objects.yaml": template})
}
func packagedFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	zip := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(zip)
	for name, raw := range files {
		data := []byte(raw)
		if e := writer.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data))}); e != nil {
			t.Fatal(e)
		}
		if _, e := writer.Write(data); e != nil {
			t.Fatal(e)
		}
	}
	if e := writer.Close(); e != nil {
		t.Fatal(e)
	}
	if e := zip.Close(); e != nil {
		t.Fatal(e)
	}
	return buffer.Bytes()
}
func TestHelmArchiveAndSealedPlanRejectSubstitution(t *testing.T) {
	if _, e := loadChart(packagedChart(t, "1.0.0", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: valid\n")); e != nil {
		t.Fatal(e)
	}
	for _, raw := range [][]byte{nil, []byte("not an archive"), make([]byte, MaxArchiveBytes+1)} {
		if _, e := loadChart(raw); e == nil {
			t.Fatal("invalid archive accepted")
		}
	}
	if _, e := parseValues([]byte("password: one\npassword: two\n")); e == nil {
		t.Fatal("duplicate values accepted")
	}
	request := Request{OperationID: "operation-a", APIServer: "https://cluster.example.test", Namespace: Namespace{UID: "ns-a"}}
	material := Material{Key: bytes.Repeat([]byte{7}, 32)}
	f := frozen{SchemaVersion: SchemaVersion, Request: request, Manifest: "password=private-canary-never-public"}
	sealed, e := sealPlan(request, material, f)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(sealed, []byte("private-canary")) {
		t.Fatal("plan not encrypted")
	}
	request.SealedPlan = sealed
	if _, e = openPlan(request, material); e != nil {
		t.Fatal(e)
	}
	request.Namespace.UID = "ns-recreated"
	if _, e = openPlan(request, material); e == nil {
		t.Fatal("plan moved to another Namespace UID")
	}
}
func TestHelmPublicDiffContainsPathsWithoutPrivateValues(t *testing.T) {
	o := Object{Before: &kubernetesinventory.Object{Data: json.RawMessage(`{"data":{"password":"old-private"}}`)}, After: &kubernetesinventory.Object{Data: json.RawMessage(`{"data":{"password":"new-private"}}`)}}
	public := publicObject(o)
	raw, _ := json.Marshal(public)
	if bytes.Contains(raw, []byte("private")) || public.Before != nil || public.After != nil {
		t.Fatal("values entered public preview")
	}
	if len(public.Changes) != 1 || public.Changes[0] != "/data/*" {
		t.Fatal(public.Changes)
	}
}
