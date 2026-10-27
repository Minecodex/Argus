package kuberneteshelm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/testkit/helmsources"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLiveHelmRepositoryAndOCIExecuteAfterSourcesAreOffline(t *testing.T) {
	ctx, cfg, api, ns := liveHelmEnvironment(t)
	sources := helmsources.Start(t)
	engine := Engine{Config: cfg, SourcePolicy: helmsources.LoopbackPolicy{}, AuthorizeWrite: func(_ context.Context, _ Request, _ Write) error { return nil }}
	type prepared struct {
		request  Request
		material Material
		preview  Result
	}
	var plans []prepared
	for _, source := range []ChartSource{
		{Kind: "repository", URL: sources.RepositoryURL, Chart: "sample", Version: "1.0.0", Digest: archiveDigest(sources.Archive)},
		{Kind: "oci", URL: sources.OCIURL, Version: "1.0.0", Digest: sources.ManifestDigest},
	} {
		r := Request{SchemaVersion: SchemaVersion, OperationID: uuid.NewString(), APIServer: cfg.Host, Namespace: Namespace{Name: ns.Name, UID: string(ns.UID)}, Name: "source-" + source.Kind, Operation: "install", TimeoutSeconds: 60, Source: &source, Access: Access{Namespaces: []Namespace{{Name: ns.Name, UID: string(ns.UID)}}, Permissions: []string{"kubernetes.configuration.manage"}}}
		m := Material{Key: bytes.Repeat([]byte{7}, 32), Values: []byte("message: source-values-private-canary"), SourceCredentials: &SourceCredentials{Username: sources.Username, Password: sources.Password, CAPEM: sources.CAPEM}}
		out, e := engine.Preview(ctx, r, m)
		if e != nil {
			t.Fatal(source.Kind, "preview", e)
		}
		if !out.DryRun || out.Accepted || out.Source == nil || out.Source.Version != "1.0.0" {
			t.Fatal("invalid source preview")
		}
		raw, _ := json.Marshal(out)
		if bytes.Contains(raw, []byte(sources.Password)) || bytes.Contains(raw, []byte("source-values-private-canary")) {
			t.Fatal("private source input leaked into native result")
		}
		if _, e := api.CoreV1().ConfigMaps(ns.Name).Get(ctx, r.Name+"-settings", metav1.GetOptions{}); !apierrors.IsNotFound(e) {
			t.Fatal("source preview wrote a real object", e)
		}
		resolved, e := ResolvePreviewMaterial(r, m, out)
		if e != nil || resolved.SourceCredentials != nil || !bytes.Equal(resolved.Chart, sources.Archive) {
			t.Fatal("resolved material", e)
		}
		r.SealedPlan = out.SealedPlan
		plans = append(plans, prepared{r, resolved, out})
	}
	sources.Close()
	for _, p := range plans {
		bad := p.material
		bad.Values = []byte("message: changed-after-approval")
		if _, e := engine.Execute(ctx, p.request, bad); !errors.Is(e, ErrConflict) {
			t.Fatal("protected values changed before the first write", e)
		}
		out, e := engine.Execute(ctx, p.request, p.material)
		if e != nil || !out.Accepted || !out.Ready {
			t.Fatal(p.request.Source.Kind, "offline-source execution", e, out.ErrorCode)
		}
		cm, e := api.CoreV1().ConfigMaps(ns.Name).Get(ctx, p.request.Name+"-settings", metav1.GetOptions{})
		if e != nil || cm.Data["message"] != "source-values-private-canary" || len(cm.Data["fixed"]) != 12 {
			t.Fatal("real source object missing or changed", e)
		}
		f, e := openPlan(p.request, p.material)
		if e != nil || !strings.Contains(f.Manifest, cm.Data["fixed"]) {
			t.Fatal("frozen random value changed")
		}
		p.material.Clear()
	}
	t.Log("actual TLS/basic-auth repository and OCI Distribution registry -> digest-bound native preview -> sources stopped -> real ConfigMaps with frozen random values and protected original Chart/values")
}
