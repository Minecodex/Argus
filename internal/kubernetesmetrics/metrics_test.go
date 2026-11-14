package kubernetesmetrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"slices"
	"strconv"
	"testing"
	"time"
)

type inventoryFixture struct {
	scope      kubernetesworkbench.Scope
	objects    map[string][]kubernetesinventory.Object
	refs       map[string]uuid.UUID
	calls      []string
	nativeErr  error
	incomplete bool
}

func (f *inventoryFixture) AccessScope(context.Context, kubernetesworkbench.Actor, uuid.UUID) (kubernetesworkbench.Scope, error) {
	return f.scope, nil
}
func (f *inventoryFixture) Get(_ context.Context, _ kubernetesworkbench.Actor, _ uuid.UUID, in kubernetesworkbench.ObjectInput) (kubernetesinventory.Object, error) {
	f.calls = append(f.calls, "get/"+in.Type)
	return kubernetesinventory.Object{}, f.nativeErr
}
func (f *inventoryFixture) List(_ context.Context, _ kubernetesworkbench.Actor, _ uuid.UUID, in kubernetesworkbench.ListInput) (kubernetesworkbench.Page, error) {
	f.calls = append(f.calls, in.Type)
	items := f.objects[in.Type]
	start := 0
	if in.Cursor != "" {
		start, _ = strconv.Atoi(in.Cursor)
	}
	end := min(len(items), start+in.Limit)
	if start > end {
		return kubernetesworkbench.Page{}, kubernetesinventory.ErrInvalid
	}
	page := kubernetesworkbench.Page{Items: slices.Clone(items[start:end]), Complete: end == len(items) && !f.incomplete, NamespaceReferences: f.refs, ObservedAt: time.Now()}
	if !page.Complete {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}
func testFixture() (Service, *inventoryFixture, kubernetesworkbench.Actor, uuid.UUID, time.Time) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	cluster := uuid.New()
	f := &inventoryFixture{scope: kubernetesworkbench.Scope{ClusterID: cluster, ClusterWide: true, ClusterVersion: 1}, objects: map[string][]kubernetesinventory.Object{}, refs: map[string]uuid.UUID{"ns-uid": uuid.New()}}
	return Service{Inventory: f, Key: bytes.Repeat([]byte{5}, 32), Now: func() time.Time { return now }}, f, kubernetesworkbench.Actor{EnterpriseID: uuid.New(), SubjectID: uuid.New(), SubjectType: "user", AuthorizationVersion: 1}, cluster, now
}
func nativeObject(kind, uid, name, namespace, owner string, created time.Time) kubernetesinventory.Object {
	d, _ := kubernetesinventory.Lookup(kind)
	raw := map[string]any{"metadata": map[string]any{"name": name, "uid": uid, "namespace": namespace}, "spec": map[string]any{}}
	if kind == "pod" {
		raw["spec"] = map[string]any{"containers": []any{map[string]any{"name": "app", "resources": map[string]any{}}}}
	}
	data, _ := json.Marshal(raw)
	o := kubernetesinventory.Object{Definition: d, UID: uid, Name: name, Namespace: namespace, CreatedAt: created, Data: data}
	if namespace != "" {
		o.NamespaceUID = "ns-uid"
	}
	if owner != "" {
		yes := true
		o.Owners = []metav1.OwnerReference{{UID: types.UID(owner), Controller: &yes}}
	}
	return o
}
func TestPrepareBatchesCurrentOwnersAndSkipsTerminalPods(t *testing.T) {
	s, f, a, cluster, now := testFixture()
	f.objects["deployment"] = []kubernetesinventory.Object{nativeObject("deployment", "dep", "api", "test", "", now.Add(-time.Hour))}
	f.objects["replicaset"] = []kubernetesinventory.Object{nativeObject("replicaset", "rs", "api-rs", "test", "dep", now)}
	f.objects["pod"] = []kubernetesinventory.Object{nativeObject("pod", "pod", "api-pod", "test", "rs", now), nativeObject("pod", "foreign", "foreign", "other", "rs", now), nativeObject("pod", "done", "done", "test", "rs", now)}
	f.objects["pod"][1].NamespaceUID = "other-uid"
	f.objects["pod"][2].Data = json.RawMessage(`{"spec":{"containers":[{"name":"app"}]},"status":{"phase":"Succeeded"}}`)
	p, e := s.Prepare(t.Context(), a, cluster, PrepareInput{List: kubernetesworkbench.ListInput{Type: "deployment", Namespaces: kubernetesworkbench.Selection{Mode: "all_authorized"}}, UIDs: []string{"dep"}})
	if e != nil {
		t.Fatal(e)
	}
	state, e := s.decode(a, cluster, p.Token)
	if e != nil || len(state.Pods) != 1 || state.Pods[0].UID != "pod" {
		t.Fatalf("wrong current owner membership: %+v %v", state.Pods, e)
	}
	if fmt.Sprint(f.calls) != "[deployment pod replicaset]" {
		t.Fatal("per-row inventory reads", f.calls)
	}
}
func TestContextCannotReplayAcrossIdentityVersionClusterOrExpiry(t *testing.T) {
	s, f, a, cluster, now := testFixture()
	f.objects["pod"] = []kubernetesinventory.Object{nativeObject("pod", "pod", "api", "test", "", now)}
	p, e := s.Prepare(t.Context(), a, cluster, PrepareInput{List: kubernetesworkbench.ListInput{Type: "pod", Namespaces: kubernetesworkbench.Selection{Mode: "all_authorized"}}, UIDs: []string{"pod"}})
	if e != nil {
		t.Fatal(e)
	}
	other := a
	other.SubjectID = uuid.New()
	if _, e = s.decode(other, cluster, p.Token); e == nil {
		t.Fatal("cross actor replay")
	}
	other = a
	other.AuthorizationVersion++
	if _, e = s.decode(other, cluster, p.Token); e == nil {
		t.Fatal("version replay")
	}
	if _, e = s.decode(a, uuid.New(), p.Token); e == nil {
		t.Fatal("cross cluster replay")
	}
	if _, e = s.decode(a, cluster, p.Token+"x"); e == nil {
		t.Fatal("tampered token")
	}
	s.Now = func() time.Time { return now.Add(3 * time.Minute) }
	if _, e = s.decode(a, cluster, p.Token); !errors.Is(e, kubernetesinventory.ErrExpired) {
		t.Fatal("expired token", e)
	}
}
func TestReadRechecksScopeAndNativeNamespaceBeforeTelemetry(t *testing.T) {
	s, f, a, cluster, now := testFixture()
	f.objects["pod"] = []kubernetesinventory.Object{nativeObject("pod", "pod", "api", "test", "", now)}
	p, e := s.Prepare(t.Context(), a, cluster, PrepareInput{List: kubernetesworkbench.ListInput{Type: "pod", Namespaces: kubernetesworkbench.Selection{Mode: "all_authorized"}}, UIDs: []string{"pod"}})
	if e != nil {
		t.Fatal(e)
	}
	f.scope.ClusterVersion++
	if _, e = s.Read(t.Context(), a, cluster, ReadInput{Token: p.Token, Metric: "cpu"}); !errors.Is(e, authorization.ErrAccessDenied) {
		t.Fatal("scope change not rejected", e)
	}
	f.scope.ClusterVersion--
	f.nativeErr = kubernetesinventory.ErrIdentityChanged
	if _, e = s.Read(t.Context(), a, cluster, ReadInput{Token: p.Token, Metric: "cpu"}); !errors.Is(e, kubernetesinventory.ErrIdentityChanged) {
		t.Fatal("namespace recreation not rejected", e)
	}
}
func TestPrepareRejectsUnsupportedAndOverBudgetInputs(t *testing.T) {
	s, _, a, c, _ := testFixture()
	for _, kind := range []string{"job", "cronjob", "configmap", "secret", "service", "controllerrevision"} {
		if _, e := s.Prepare(t.Context(), a, c, PrepareInput{List: kubernetesworkbench.ListInput{Type: kind}, UIDs: []string{"pod"}}); e == nil {
			t.Fatal(kind)
		}
	}
	if _, e := s.Prepare(t.Context(), a, c, PrepareInput{List: kubernetesworkbench.ListInput{Type: "pod"}, UIDs: []string{"pod", "pod"}}); e == nil {
		t.Fatal("duplicate UID")
	}
}
func TestEffectiveConfigurationDoesNotTreatPartialLimitsAsPodLimit(t *testing.T) {
	s, f, _, _, now := testFixture()
	_ = s
	_ = f
	q := func(text string) resource.Quantity { return resource.MustParse(text) }
	native := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "a", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: q("100m")}, Limits: corev1.ResourceList{corev1.ResourceCPU: q("1")}}}, {Name: "b", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: q("200m")}}}}}}
	data, _ := json.Marshal(native)
	p := podFrom(kubernetesinventory.Object{UID: "p", Data: data, CreatedAt: now}, nil)
	if p.Budgets["cpu"].Limit != nil || !p.Budgets["cpu"].Partial || p.Budgets["cpu"].Request == nil {
		t.Fatal("partial limit claimed as a complete ceiling", p.Budgets)
	}
}
func TestTrendKeepsZerosGapsAndHistoricalDenominators(t *testing.T) {
	s, _, _, _, now := testFixture()
	r := row{UID: "parent", Created: now.Add(-time.Hour), Budgets: map[string]budget{"cpu": {Request: number(100), Limit: number(200)}}}
	state := preparedState{Type: "deployment", Complete: true, Pods: []pod{{UID: "pod", Parents: []string{"parent"}, Created: now.Add(-time.Hour), Containers: 1}}}
	used := map[string]series{"pod/used": {now.Unix() - 60: 0, now.Unix(): .5}, "pod/sample_time": {now.Unix() - 60: float64(now.Unix() - 60), now.Unix(): float64(now.Unix())}}
	conf := map[string]series{"pod/request": {now.Unix(): .1}, "pod/request_count": {now.Unix(): 1}}
	tr := s.buildTrend(state, r, "cpu", used, conf, now.Add(-time.Minute), now)
	if tr.State != "ready" || tr.Basis != "request" || tr.Points[0].Value == nil || *tr.Points[0].Value != 0 || tr.Points[1].Value != nil || tr.Points[0].Limit != nil || tr.Points[0].Request != nil || tr.Points[2].Request == nil {
		t.Fatal("zero/gap/historical config changed", tr)
	}
	if *tr.Points[2].Value / *tr.Points[2].Request <= 1 {
		t.Fatal("request utilization clamped")
	}
	state.Complete = false
	tr = s.buildTrend(state, r, "cpu", used, conf, now.Add(-time.Minute), now)
	if tr.State != "partial" || tr.Points[2].Value != nil {
		t.Fatal("partial total presented as exact", tr)
	}
}
func TestSeriesParserPreservesFiniteValuesAndRejectsMalformedProtocol(t *testing.T) {
	v, e := parseSeries([]map[string]any{{"metric": map[string]string{"resource_k8s_pod_uid": "pod", "argus_trend_component": "used"}, "values": []any{[]any{120, "0"}, []any{150, "NaN"}}}})
	if e != nil || len(v["pod/used"]) != 1 {
		t.Fatal(v, e)
	}
	if _, e = parseSeries([]map[string]any{{"metric": map[string]string{}, "values": []any{[]any{"wrong", "1"}}}}); e == nil {
		t.Fatal("invalid timestamp accepted")
	}
}

func TestMetricSourceRootNeverExpandsNamespaceOnlyAuthority(t *testing.T) {
	cluster, ns := uuid.New(), uuid.New()
	state := preparedState{Cluster: cluster, Rows: []row{{UID: "p", NamespaceID: ns}}}
	refs := metricResources(state)
	if len(refs) != 1 || refs[0].Type != "kubernetes_namespace" || refs[0].ID != ns {
		t.Fatal("namespace scope widened", refs)
	}
	state.ClusterWide = true
	refs = metricResources(state)
	if len(refs) != 1 || refs[0].Type != "kubernetes_cluster" || refs[0].ID != cluster {
		t.Fatal("owned cluster root missing", refs)
	}
}

func TestStaticPodAliasUsesNamespaceAndNameIdentityAndCreationBoundary(t *testing.T) {
	s, _, _, _, now := testFixture()
	p := pod{UID: "api-uid", Alias: "static-hash", Name: "static-node-a", Namespace: "system", Parents: []string{"parent"}, Created: now.Add(-time.Hour), Containers: 1}
	state := preparedState{Type: "pod", Complete: true, Pods: []pod{p}}
	r := row{UID: "parent", Created: p.Created}
	key := podKey(p, p.Alias)
	used := map[string]series{key + "/used": {now.Unix(): .1}, key + "/sample_time": {now.Unix(): float64(now.Unix())}, "other/static-node-b/static-hash/used": {now.Unix(): 99}}
	tr := s.buildTrend(state, r, "cpu", used, nil, now, now)
	if tr.Points[0].Value == nil || *tr.Points[0].Value != .1 {
		t.Fatal("static alias missing or crossed namespace/name", tr)
	}
	state.Pods[0].Created = now.Add(-10 * time.Second)
	used[key+"/sample_time"][now.Unix()] = float64(now.Add(-20 * time.Second).Unix())
	tr = s.buildTrend(state, r, "cpu", used, nil, now, now)
	if tr.Points[0].Value != nil {
		t.Fatal("pre-recreation lookback sample attached to new object", tr)
	}
}
