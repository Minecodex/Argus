package dashboard

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"testing"
	"time"
)

type overviewInventoryFixture struct {
	calls    int
	input    kubernetesworkbench.SnapshotInput
	snapshot kubernetesworkbench.Snapshot
	err      error
}

func (f *overviewInventoryFixture) AccessScope(context.Context, kubernetesworkbench.Actor, uuid.UUID) (kubernetesworkbench.Scope, error) {
	return kubernetesworkbench.Scope{}, nil
}
func (f *overviewInventoryFixture) Get(context.Context, kubernetesworkbench.Actor, uuid.UUID, kubernetesworkbench.ObjectInput) (kubernetesinventory.Object, error) {
	return kubernetesinventory.Object{}, nil
}
func (f *overviewInventoryFixture) Snapshot(_ context.Context, _ kubernetesworkbench.Actor, _ uuid.UUID, input kubernetesworkbench.SnapshotInput) (kubernetesworkbench.Snapshot, error) {
	f.calls++
	f.input = input
	return f.snapshot, f.err
}
func TestOperationalRoundCoalescesInventoryAndDoesNotMutateSource(t *testing.T) {
	cluster := uuid.New()
	now := time.Now().UTC()
	f := &overviewInventoryFixture{snapshot: kubernetesworkbench.Snapshot{ClusterID: cluster, ClusterWide: true, Complete: true, ObservedAt: now, Errors: map[string]string{}, Counts: map[string]map[string]int{"pod": {"total": 2, "healthy": 1, "failed": 1}}, Resources: []kubernetesworkbench.SnapshotResource{
		{ResourceReference: kubernetesworkbench.ResourceReference{UID: "one", Type: "pod", Name: "one"}, Summary: kubernetesinventory.Summary{State: "healthy", Fields: map[string]string{}}},
		{ResourceReference: kubernetesworkbench.ResourceReference{UID: "two", Type: "pod", Name: "two"}, Summary: kubernetesinventory.Summary{State: "failed", Fields: map[string]string{}}},
	}, Relations: []kubernetesworkbench.ResourceRelation{}}}
	runtime := Runtime{Inventory: f}
	round := &operationalRound{}
	result := Execution{Resources: []ResourceScope{{Type: "kubernetes_cluster", ID: cluster}}, From: now.Add(-time.Hour), To: now}
	panel := operationsOverviewTemplate().Spec.Panels[0]
	panel.DataSource.States = []string{"failed"}
	value, err := runtime.executeOperational(t.Context(), Actor{}, panel, result, round)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := value.Data.(kubernetesworkbench.Snapshot)
	if len(snapshot.Resources) != 1 || snapshot.Counts["pod"]["total"] != 1 {
		t.Fatal("filtered counts disagree", snapshot)
	}
	panel.DataSource.States = nil
	value, err = runtime.executeOperational(t.Context(), Actor{}, panel, result, round)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || len(value.Data.(kubernetesworkbench.Snapshot).Resources) != 2 {
		t.Fatal("duplicate reads or cross-panel mutation")
	}
	result.KubernetesScope = &dashboardparams.KubernetesScope{ClusterID: uuid.New()}
	value, err = runtime.executeOperational(t.Context(), Actor{}, panel, result, round)
	if err != nil || value.Status != "error" || f.calls != 1 {
		t.Fatal("explicit foreign cluster scope accepted")
	}
	raw, _ := json.Marshal(value.Data)
	if len(raw) > 4<<20 {
		t.Fatal("unbounded output")
	}
}
func TestOperationalGraphNeverSilentlyTruncates(t *testing.T) {
	cluster := uuid.New()
	rows := []kubernetesworkbench.SnapshotResource{{ResourceReference: kubernetesworkbench.ResourceReference{UID: "owner", Type: "deployment"}, Summary: kubernetesinventory.Summary{State: "healthy"}}}
	edges := []kubernetesworkbench.ResourceRelation{}
	for i := 0; i < 201; i++ {
		id := uuid.NewString()
		rows = append(rows, kubernetesworkbench.SnapshotResource{ResourceReference: kubernetesworkbench.ResourceReference{UID: id, Type: "pod"}, Summary: kubernetesinventory.Summary{State: "healthy"}})
		edges = append(edges, kubernetesworkbench.ResourceRelation{From: "owner", To: id, Kind: "owns"})
	}
	f := &overviewInventoryFixture{snapshot: kubernetesworkbench.Snapshot{ClusterID: cluster, Resources: rows, Relations: edges, Complete: true, Errors: map[string]string{}, Counts: map[string]map[string]int{}}}
	panel := operationsOverviewTemplate().Spec.Panels[0]
	panel.Type = "resource_graph"
	value, err := (Runtime{Inventory: f}).executeOperational(t.Context(), Actor{}, panel, Execution{Resources: []ResourceScope{{Type: "kubernetes_cluster", ID: cluster}}}, &operationalRound{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := value.Data.(kubernetesworkbench.Snapshot)
	if snapshot.Complete || len(snapshot.Resources) > 0 || snapshot.Errors["graph"] != "KUBERNETES_RESULT_BUDGET_EXCEEDED" {
		t.Fatal(snapshot)
	}
}
