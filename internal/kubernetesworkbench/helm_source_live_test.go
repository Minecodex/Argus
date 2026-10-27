package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/testkit/helmsources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Native API/source adapter with real PG, approval, private envelopes, worker
// and Kubernetes objects. Connector delivery and HTTP have separate gates.
type liveHelmSourceBackend struct {
	*helmFixtureBackend
	engine kuberneteshelm.Engine
	t      *testing.T
}

func (b *liveHelmSourceBackend) PreviewHelm(ctx context.Context, c db.KubernetesCluster, r kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	m, e := b.service.Credentials.LoadHelmInput(ctx, c.EnterpriseID, c.ID, r, true, "server", c.ID.String())
	if e != nil {
		return kuberneteshelm.Result{}, e
	}
	defer m.Clear()
	out, e := b.engine.Preview(ctx, r, m)
	b.t.Logf("native source preview: error=%v sealed_bytes=%d encrypted_chart_bytes=%d objects=%d", e, len(out.SealedPlan), len(out.EncryptedChart), len(out.Objects))
	return out, e
}
func (b *liveHelmSourceBackend) ExecuteHelm(ctx context.Context, c db.KubernetesCluster, r kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	m, e := b.service.Credentials.LoadHelmInput(ctx, c.EnterpriseID, c.ID, r, false, "worker", c.ID.String())
	if e != nil {
		return kuberneteshelm.Result{}, e
	}
	defer m.Clear()
	if m.SourceCredentials != nil {
		return kuberneteshelm.Result{}, errors.New("origin credentials retained for execution")
	}
	engine := b.engine
	engine.AuthorizeWrite = func(ctx context.Context, r kuberneteshelm.Request, w kuberneteshelm.Write) error {
		return b.service.ValidateHelmWrite(ctx, c.EnterpriseID, c.ID, uuid.MustParse(r.OperationID), r, w)
	}
	return engine.Execute(ctx, r, m)
}
func (b *liveHelmSourceBackend) ObserveHelm(ctx context.Context, c db.KubernetesCluster, r kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	m, e := b.service.Credentials.LoadHelmInput(ctx, c.EnterpriseID, c.ID, r, false, "worker", c.ID.String())
	if e != nil {
		return kuberneteshelm.Result{}, e
	}
	defer m.Clear()
	return b.engine.Observe(ctx, r, m)
}

func TestLivePostgresHelmSourceApprovalErasesOriginCredentialsAndPinsBytes(t *testing.T) {
	// Establish both explicit prerequisites before creating any external resource.
	if os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL") == "" || os.Getenv("ARGUS_HELM_SOURCE_REGISTRY") != "1" || os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT") == "" {
		t.Skip("requires owned PostgreSQL, source registry and Kubernetes context")
	}
	ctx, cfg, api, ns := helmsources.Namespace(t)
	sources := helmsources.Start(t)
	type pendingSource struct {
		fixture *mutationFixture
		action  db.PendingAction
		input   HelmInput
	}
	var actions []pendingSource
	for _, source := range []kuberneteshelm.ChartSource{{Kind: "repository", URL: sources.RepositoryURL, Chart: "sample", Version: "1.0.0"}, {Kind: "oci", URL: sources.OCIURL, Version: "1.0.0", Digest: sources.ManifestDigest}} {
		f, b, input := newHelmFixture(t)
		f.exec("UPDATE kubernetes_clusters SET api_server=$2 WHERE id=$1", f.cluster, cfg.Host)
		f.exec("UPDATE kubernetes_namespace_resources SET name=$2,namespace_uid=$3 WHERE id=$1", f.namespace, ns.Name, string(ns.UID))
		f.service.Backend = &liveHelmSourceBackend{helmFixtureBackend: b, engine: kuberneteshelm.Engine{Config: cfg, SourcePolicy: helmsources.LoopbackPolicy{}}, t: t}
		input.Name = "approved-" + source.Kind
		input.Source = &source
		private := kuberneteshelm.Material{Values: []byte("message: protected-source-values"), SourceCredentials: &kuberneteshelm.SourceCredentials{Username: sources.Username, Password: sources.Password, CAPEM: sources.CAPEM}}
		action, e := f.service.PreviewHelmProtected(ctx, f.actor, f.cluster, input, private, uuid.NewString())
		if e != nil {
			t.Fatal(source.Kind, "domain preview", e)
		}
		for _, raw := range [][]byte{action.Preview, action.Diff} {
			if bytes.Contains(raw, []byte(sources.Password)) || bytes.Contains(raw, []byte("protected-source-values")) {
				t.Fatal("private source leaked into approval")
			}
		}
		rows, e := f.service.Store.Queries.ListKubernetesOperations(ctx, db.ListKubernetesOperationsParams{EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, ClusterWide: true, NamespaceIds: []uuid.UUID{}, PageLimit: 10})
		if e != nil || len(rows) != 1 {
			t.Fatal(e, len(rows))
		}
		var plan helmPlan
		if json.Unmarshal(rows[0].PrivatePlan, &plan) != nil || plan.Source == nil || plan.Source.Digest == "" {
			t.Fatal("source digest not in immutable operation")
		}
		material, e := f.service.Credentials.LoadHelmInput(ctx, f.actor.EnterpriseID, f.cluster, plan.Request, true, "server", f.cluster.String())
		if e != nil || material.SourceCredentials != nil || !bytes.Equal(material.Chart, sources.Archive) {
			t.Fatal("source envelope was not replaced by pinned chart", e)
		}
		material.Clear()
		actions = append(actions, pendingSource{f, action, input})
	}
	sources.Close()
	for _, item := range actions {
		f := item.fixture
		id := confirmHelm(f, item.action)
		if e := (Worker{Service: f.service}).execute(ctx, operationTask{f.actor.EnterpriseID, id}); e != nil {
			t.Fatal(e)
		}
		if row := f.operation(id); row.Status != "succeeded" {
			t.Fatal("source operation", row.Status, row.ErrorCode)
		}
		cm, e := api.CoreV1().ConfigMaps(ns.Name).Get(ctx, item.input.Name+"-settings", metav1.GetOptions{})
		if e != nil || cm.Data["message"] != "protected-source-values" {
			t.Fatal("actual approved ConfigMap", e)
		}
	}
	t.Log("actual repository/OCI source -> encrypted staged input -> immutable PG preview/approval -> source auth erased -> sources offline -> worker/per-write grants -> real ConfigMaps")
}
