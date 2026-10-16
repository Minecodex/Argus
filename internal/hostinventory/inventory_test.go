package hostinventory

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type metadataTransport func(*http.Request) (*http.Response, error)

func (fn metadataTransport) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestCloudRequiresActualIdentityAndIMDSv2(t *testing.T) {
	previous := metadataClient
	defer func() { metadataClient = previous }()
	calls := 0
	metadataClient = &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body := ""
		code := 200
		switch r.URL.Path {
		case "/latest/api/token":
			if r.Method != "PUT" || r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") == "" {
				t.Fatal("missing token request")
			}
			body = "local-token"
		case "/latest/dynamic/instance-identity/document":
			if r.Header.Get("X-aws-ec2-metadata-token") != "local-token" {
				t.Fatal("token was not bound to request")
			}
			body = `{"instanceId":"i-real","region":"us-east-1","availabilityZone":"us-east-1a"}`
		default:
			t.Fatalf("unexpected metadata path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	cloud := collectCloud(context.Background())
	if cloud.Provider != "aws" || cloud.InstanceID != "i-real" || calls != 2 {
		t.Fatalf("unexpected cloud %+v calls=%d", cloud, calls)
	}
}
func TestCloudDoesNotInventProviderFromUnavailableMetadata(t *testing.T) {
	previous := metadataClient
	defer func() { metadataClient = previous }()
	metadataClient = &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	cloud := collectCloud(context.Background())
	if cloud.Status != "not_detected" || cloud.Provider != "" || cloud.InstanceID != "" {
		t.Fatalf("fabricated cloud %+v", cloud)
	}
}
func TestInventoryRejectsMalformedFutureAndOversizedSnapshots(t *testing.T) {
	valid := Snapshot{SchemaVersion: SchemaVersion, CollectedAt: time.Now().UTC(), Scope: "machine", Disks: []Disk{}, Networks: []Network{}, GPUs: []GPU{}, Issues: []Issue{}, Cloud: Cloud{Status: "not_detected"}}
	raw, _ := json.Marshal(valid)
	if _, err := Decode(raw); err != nil {
		t.Fatal(err)
	}
	valid.CollectedAt = time.Now().Add(time.Hour)
	future, _ := json.Marshal(valid)
	for _, raw := range [][]byte{[]byte(`{"schema_version":"wrong"}`), future, make([]byte, MaxBytes+1)} {
		if _, err := Decode(raw); err == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
}
