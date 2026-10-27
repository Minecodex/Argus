package monitoring_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/collectionclaim"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetrybinding"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type actualNodeBindingFixture struct {
	t *testing.T
	s struct {
		Store   *postgres.Store
		Actions resource.PendingActionService
	}
	a telemetry.Actor
}

func newActualNodeBindingFixture(t *testing.T) *actualNodeBindingFixture {
	t.Helper()
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "actual_node_binding", filepath.Join("..", "..", "migrations", "postgresql"))
	f := &actualNodeBindingFixture{t: t, a: telemetry.Actor{EnterpriseID: uuid.New(), SubjectID: uuid.New(), SubjectType: "user", AuthorizationVersion: 1}}
	f.s.Store = store
	department := uuid.New()
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Owned actual Node binding',$2,'UTC')", f.a.EnterpriseID, f.a.EnterpriseID.String())
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Owned Node binding')", department, f.a.EnterpriseID)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Owned Node operator')", f.a.SubjectID, f.a.EnterpriseID, department, f.a.SubjectID.String())
	key := bytes.Repeat([]byte{7}, 32)
	f.s.Actions = resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}
	if err := audit.InitializeChain(t.Context(), store.Queries, "enterprise", uuid.NullUUID{UUID: f.a.EnterpriseID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *actualNodeBindingFixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.s.Store.Pool.Exec(f.t.Context(), query, args...); err != nil {
		f.t.Fatal(err)
	}
}

// Registration and the identity-drift fault are controlled. Baseline Node facts
// use the live API; preview/confirmation execute the production domain transaction.
// This does not claim an actual Kubernetes Node replacement.
func TestPostgresActualNodeBindingConfirmationAndControlledIdentityDrift(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_NODE_BINDING") != "1" {
		t.Skip("explicit owned live Node and PostgreSQL execution required")
	}
	var receipt struct {
		Passed       bool   `json:"passed"`
		NodeName     string `json:"node_name"`
		NodeUID      string `json:"node_uid"`
		MachineID    string `json:"machine_id"`
		BootID       string `json:"boot_id"`
		InternalIP   string `json:"internal_ip"`
		SameMachine  bool   `json:"same_actual_machine_and_boot_identity"`
		NoDoubleRead bool   `json:"host_canonical_symlink_did_not_double_read_kubernetes_logs"`
	}
	body, err := os.ReadFile(os.Getenv("ARGUS_PLAN8_NODE_COEXIST_EVIDENCE"))
	if err != nil || json.Unmarshal(body, &receipt) != nil || !receipt.Passed || !receipt.SameMachine || !receipt.NoDoubleRead {
		t.Fatal("actual same-node native observation required", err)
	}
	if receipt.NodeName != "plan8-drain-completion-20261021-control-plane" || os.Getenv("ARGUS_PLAN8_COEXIST_CONTEXT") != "kind-plan8-drain-completion-20261021" {
		t.Fatal("exact owned live node required")
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: os.Getenv("ARGUS_PLAN8_COEXIST_CONTEXT")}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	node, err := api.CoreV1().Nodes().Get(ctx, receipt.NodeName, metav1.GetOptions{})
	if err != nil || string(node.UID) != receipt.NodeUID || node.Status.NodeInfo.MachineID != receipt.MachineID || node.Status.NodeInfo.BootID != receipt.BootID {
		t.Fatal("actual node no longer matches the native observations", err)
	}
	f := newActualNodeBindingFixture(t)
	host, cluster := uuid.New(), uuid.New()
	f.exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,$3,$4,22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, f.a.EnterpriseID, receipt.NodeName, receipt.InternalIP)
	f.exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'Owned actual node binding',$3,'in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, f.a.EnterpriseID, config.Host)
	for kind, id := range map[string]uuid.UUID{"host": host, "kubernetes_cluster": cluster} {
		f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,$4,$5)", uuid.New(), f.a.EnterpriseID, f.a.SubjectID, kind, id)
	}
	identityFault := false
	observe := func() db.KubernetesNodeHostBinding {
		t.Helper()
		values, e := telemetrybinding.Collect(ctx, api)
		if e != nil {
			t.Fatal(e)
		}
		selected := []telemetrybinding.NodeEvidence{}
		for _, v := range values {
			if v.NodeUID == receipt.NodeUID {
				selected = append(selected, v)
			}
		}
		if len(selected) != 1 {
			t.Fatal("actual node identity disappeared")
		}
		if identityFault {
			selected[0].ProviderID += "#controlled-identity-drift"
		}
		if e = telemetrybinding.Upsert(ctx, f.s.Store.Queries, f.a.EnterpriseID, cluster, selected); e != nil {
			t.Fatal(e)
		}
		rows, e := f.s.Store.Queries.ListKubernetesNodeHostBindings(ctx, db.ListKubernetesNodeHostBindingsParams{EnterpriseID: f.a.EnterpriseID, KubernetesClusterID: cluster})
		if e != nil || len(rows) != 1 {
			t.Fatal("actual node binding count", len(rows), e)
		}
		return rows[0]
	}
	weak := observe()
	if weak.Status == "verified" || !weak.HostID.Valid || weak.HostID.UUID != host {
		t.Fatal("IP candidate acquired automatic Host authority")
	}
	service := telemetry.Service{Store: f.s.Store, Actions: f.s.Actions}
	actor := telemetry.Actor{EnterpriseID: f.a.EnterpriseID, SubjectID: f.a.SubjectID, SubjectType: "user", AuthorizationVersion: 1}
	if _, e := service.PreviewBinding(ctx, actor, weak.ID, host, weak.Version, uuid.NewString()); e == nil {
		t.Fatal("empty caller resource context authorized a node binding")
	}
	// This direct domain call receives the same explicit resource context that
	// HTTP resolves from the two grants installed above; empty remains denied.
	actor.AuthorizedResourceIDs = []uuid.UUID{host, cluster}
	extension := telemetry.ActionExtension{}
	action, err := service.PreviewBinding(ctx, actor, weak.ID, host, weak.Version, uuid.NewString())
	if err != nil {
		t.Fatal("actual domain binding preview", err)
	}
	if _, err = f.s.Actions.Confirm(ctx, actor.SubjectID.String(), actor.EnterpriseID, 1, action.ActionRef, uuid.NewString(), extension.RevalidateAction, extension.CommitAction); err != nil {
		t.Fatal("actual domain binding confirmation", err)
	}
	verified, err := f.s.Store.Queries.GetKubernetesNodeHostBinding(ctx, db.GetKubernetesNodeHostBindingParams{ID: weak.ID, EnterpriseID: f.a.EnterpriseID})
	if err != nil || verified.Status != "verified" {
		t.Fatal("confirmation did not establish verified identity", err)
	}
	claim := configbundle.Collection{ID: uuid.NewString(), Integration: "kubernetes_node_metrics", TargetType: "kubernetes_cluster", TargetID: cluster.String(), Enabled: true, Revision: 1, AssignmentEpoch: 1}
	plan := func(binding db.KubernetesNodeHostBinding) string {
		t.Helper()
		rows, e := collectionclaim.Plan(db.CollectorInstance{ID: uuid.New(), ResourceType: "kubernetes_cluster", ResourceID: cluster}, nil, nil, []configbundle.Collection{claim}, []db.KubernetesNodeHostBinding{binding})
		if e != nil || len(rows) != 1 {
			t.Fatal("claim identity plan", e)
		}
		return rows[0].Physical
	}
	if plan(weak) == "host:"+host.String() || plan(verified) != "host:"+host.String() {
		t.Fatal("claim aliases did not follow explicit verified identity")
	}
	stale, err := service.PreviewBinding(ctx, actor, verified.ID, host, verified.Version, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	// The actual API forbids changing an established providerID. Inject only
	// the trusted-report fault here; do not weaken Kubernetes validation.
	identityFault = true
	drift := observe()
	if drift.Status == "verified" || plan(drift) == "host:"+host.String() {
		t.Fatal("strong identity change retained Host authority")
	}
	if _, err = f.s.Actions.Confirm(ctx, actor.SubjectID.String(), actor.EnterpriseID, 1, stale.ActionRef, uuid.NewString(), extension.RevalidateAction, extension.CommitAction); err == nil {
		t.Fatal("stale preview confirmed after controlled strong identity report change")
	}
	identityFault = false
	if restored := observe(); restored.Status == "verified" {
		t.Fatal("restored metadata silently renewed old approval")
	}
}
