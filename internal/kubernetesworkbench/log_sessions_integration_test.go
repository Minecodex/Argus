package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type sessionLogBackend struct {
	inventoryFixtureBackend
	pod       corev1.Pod
	raw       []byte
	calls     int
	afterRead func()
}

func (b *sessionLogBackend) GetInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	if q.Type != "pod" || q.Namespace != "orders" || q.NamespaceUID != "orders-uid" || q.Name != b.pod.Name || q.UID != string(b.pod.UID) {
		return kubernetesinventory.Object{}, kubernetesinventory.ErrIdentityChanged
	}
	raw, _ := json.Marshal(b.pod)
	var u unstructured.Unstructured
	_ = u.UnmarshalJSON(raw)
	d, _ := kubernetesinventory.Lookup("pod")
	return kubernetesinventory.Project(d, u, q.NamespaceUID, time.Now())
}
func (b *sessionLogBackend) ReadPinnedPodLogs(_ context.Context, _ db.KubernetesCluster, _ uuid.UUID, q kubernetespodlogs.Query) (kubernetespodlogs.Batch, error) {
	b.calls++
	pin, err := kubernetespodlogs.PinPod("orders-uid", &b.pod, q.Pin.Container, q.Pin.Previous)
	if err != nil || pin != q.Pin {
		return kubernetespodlogs.Batch{}, kubernetesinventory.ErrIdentityChanged
	}
	result, err := kubernetespodlogs.Continue(b.raw, q, time.Now())
	if b.afterRead != nil {
		b.afterRead()
	}
	return result, err
}

func newLogFixture(t *testing.T) (*mutationFixture, *sessionLogBackend, uuid.UUID, LogSessionInput) {
	t.Helper()
	f := newMutationFixture(t)
	httpID := uuid.New()
	f.exec("INSERT INTO sessions(id,token_hash,csrf_hash,audience,user_id,enterprise_id,department_id,authorization_version,locale,idle_expires_at,absolute_expires_at,last_seen_at) VALUES($1,decode(repeat(replace($1::uuid::text,'-',''),2),'hex'),decode(repeat('cd',32),'hex'),'enterprise',$2,$3,(SELECT department_id FROM enterprise_users WHERE id=$2),1,'zh-CN',now()+interval '1 hour',now()+interval '1 hour',now())", httpID, f.actor.SubjectID, f.actor.EnterpriseID)
	b := &sessionLogBackend{pod: corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}, ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "orders", UID: "pod-uid", ResourceVersion: "1"}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "main", ContainerID: "containerd://current", RestartCount: 1, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ContainerID: "containerd://previous"}}}}}}}
	now := time.Now().UTC().Add(-2 * time.Second).Format(time.RFC3339Nano)
	b.raw = []byte(now + " duplicate\n" + now + " duplicate\n" + now + " password=log-canary-never-visible\n")
	f.service.Backend = b
	t.Cleanup(func() {
		_, _ = f.service.Store.Pool.Exec(context.Background(), "DELETE FROM kubernetes_log_sessions WHERE enterprise_id=$1", f.actor.EnterpriseID)
		_, _ = f.service.Store.Pool.Exec(context.Background(), "DELETE FROM sessions WHERE id=$1", httpID)
	})
	return f, b, httpID, LogSessionInput{NamespaceID: f.namespace, Pod: "api", PodUID: "pod-uid", Container: "main", TailLines: 100, SinceSeconds: 300}
}
func grantOperationalLogs(f *mutationFixture) {
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'kubernetes.operational_logs.read' FROM role_bindings WHERE enterprise_id=$1 AND subject_id=$2 ON CONFLICT DO NOTHING", f.actor.EnterpriseID, f.actor.SubjectID)
}
func openFixtureLog(t *testing.T, f *mutationFixture, httpID uuid.UUID, in LogSessionInput) LogTicket {
	t.Helper()
	ticket, err := f.service.CreateLogSession(t.Context(), f.actor, httpID, f.cluster, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AttachLogSession(t.Context(), f.actor, httpID, f.cluster, ticket.Session.ID, ticket.Ticket); err != nil {
		t.Fatal(err)
	}
	return ticket
}

func TestPostgresPodLogSessionsBindTicketsAndRecheckRevocation(t *testing.T) {
	f, b, httpID, in := newLogFixture(t)
	ctx := t.Context()
	if _, err := f.service.CreateLogSession(ctx, f.actor, httpID, f.cluster, in); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatalf("cluster/namespace scope implied log permission: %v", err)
	}
	grantOperationalLogs(f)
	ticket, err := f.service.CreateLogSession(ctx, f.actor, httpID, f.cluster, in)
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.service.Store.Queries.GetKubernetesLogSession(ctx, db.GetKubernetesLogSessionParams{EnterpriseID: f.actor.EnterpriseID, ID: ticket.Session.ID})
	if err != nil || string(row.TicketHash) == ticket.Ticket || len(row.TicketHash) != 32 {
		t.Fatal("raw ticket persisted or missing hash")
	}
	if _, err = f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, row.ID, ""); !errors.Is(err, kubernetesinventory.ErrMutationConflict) {
		t.Fatal("unattached read was accepted", err)
	}
	if _, err = f.service.AttachLogSession(ctx, f.actor, uuid.New(), f.cluster, row.ID, ticket.Ticket); err == nil {
		t.Fatal("ticket moved to another HTTP session")
	}
	if _, err = f.service.AttachLogSession(ctx, f.actor, httpID, f.cluster, row.ID, strings.Repeat("a", 43)); err == nil {
		t.Fatal("incorrect ticket accepted")
	}
	if _, err = f.service.AttachLogSession(ctx, f.actor, httpID, f.cluster, row.ID, ticket.Ticket); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AttachLogSession(ctx, f.actor, httpID, f.cluster, row.ID, ticket.Ticket); err == nil {
		t.Fatal("ticket replay accepted")
	}
	first, err := f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, row.ID, "")
	if err != nil || strings.Count(first.Content, "duplicate") != 2 || strings.Contains(first.Content, "log-canary-never-visible") || !first.Redacted {
		t.Fatalf("incremental content/redaction: %+v %v", first, err)
	}
	if _, err = f.service.GetLogSession(ctx, f.actor, httpID, f.cluster, row.ID); err != nil {
		t.Fatal("paused session heartbeat", err)
	}
	empty, err := f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, row.ID, first.NextCursor)
	if err != nil || empty.Content != "" {
		t.Fatalf("increment repeated old data: %v %q", err, empty.Content)
	}
	another := openFixtureLog(t, f, httpID, in)
	calls := b.calls
	if _, err = f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, another.Session.ID, first.NextCursor); err == nil || b.calls != calls {
		t.Fatal("cursor moved to another session")
	}
	var query kubernetespodlogs.Query
	_ = json.Unmarshal(row.Binding, &query)
	if err = ValidateLogDelivery(ctx, f.service.Store.Queries, f.actor.EnterpriseID, f.cluster, row.ID, query); err != nil {
		t.Fatal(err)
	}
	forged := query
	forged.Pin.PodUID = "foreign"
	if err = ValidateLogDelivery(ctx, f.service.Store.Queries, f.actor.EnterpriseID, f.cluster, row.ID, forged); err == nil {
		t.Fatal("Gateway accepted a substituted Pod")
	}
	b.afterRead = func() {
		f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_id=$2", f.actor.EnterpriseID, f.namespace)
	}
	data, err := f.service.ReadLogSession(ctx, f.actor, httpID, f.cluster, row.ID, first.NextCursor)
	if err == nil || data.Content != "" {
		t.Fatal("grant-only revocation after read leaked data")
	}
	if _, err = f.service.GetLogSession(ctx, f.actor, httpID, f.cluster, row.ID); err == nil {
		t.Fatal("paused session survived revocation")
	}
	if err = ValidateLogDelivery(ctx, f.service.Store.Queries, f.actor.EnterpriseID, f.cluster, another.Session.ID, query); err == nil {
		t.Fatal("queued command survived resource grant revocation")
	}
	row, err = f.service.Store.Queries.GetKubernetesLogSession(ctx, db.GetKubernetesLogSessionParams{EnterpriseID: f.actor.EnterpriseID, ID: row.ID})
	if err != nil || row.Status != "invalidated" {
		t.Fatal("revoked session remained active")
	}
}

func TestPostgresPodLogSessionCapacityAndRetirement(t *testing.T) {
	f, _, httpID, in := newLogFixture(t)
	grantOperationalLogs(f)
	var wait sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		wait.Go(func() {
			_, err := f.service.CreateLogSession(t.Context(), f.actor, httpID, f.cluster, in)
			results <- err
		})
	}
	wait.Wait()
	close(results)
	passed, limited := 0, 0
	for err := range results {
		if err == nil {
			passed++
		} else if errors.Is(err, kubernetesinventory.ErrBudget) {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if passed != 10 || limited != 2 {
		t.Fatalf("concurrent session cap bypassed: passed=%d limited=%d", passed, limited)
	}
	f.exec("UPDATE kubernetes_log_sessions SET ticket_expires_at=now()-interval '1 second' WHERE enterprise_id=$1", f.actor.EnterpriseID)
	if err := f.service.Store.Queries.RetireKubernetesLogSessions(t.Context()); err != nil {
		t.Fatal(err)
	}
	var invalid int
	if err := f.service.Store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM kubernetes_log_sessions WHERE enterprise_id=$1 AND status='invalidated' AND ticket_hash IS NULL AND NOT(binding ? 'continuity_key')", f.actor.EnterpriseID).Scan(&invalid); err != nil || invalid != 10 {
		t.Fatal("expired capabilities retained", err, invalid)
	}
	f.exec("UPDATE kubernetes_log_sessions SET expires_at=now()-interval '25 hours' WHERE enterprise_id=$1", f.actor.EnterpriseID)
	if err := f.service.Store.Queries.DeleteRetiredKubernetesLogSessions(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM kubernetes_log_sessions WHERE enterprise_id=$1", f.actor.EnterpriseID).Scan(&invalid); err != nil || invalid != 0 {
		t.Fatal("retired session rows remained", err, invalid)
	}
}

func TestPostgresPodLogSessionsRejectRestartExpiryAndLogout(t *testing.T) {
	for _, kind := range []string{"restart", "namespace", "cluster", "logout", "ticket-expiry", "session-expiry", "closed"} {
		t.Run(kind, func(t *testing.T) {
			f, b, httpID, in := newLogFixture(t)
			grantOperationalLogs(f)
			ticket, err := f.service.CreateLogSession(t.Context(), f.actor, httpID, f.cluster, in)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "ticket-expiry" {
				f.exec("UPDATE kubernetes_log_sessions SET ticket_expires_at=now()-interval '1 second' WHERE id=$1", ticket.Session.ID)
				if _, err = f.service.AttachLogSession(t.Context(), f.actor, httpID, f.cluster, ticket.Session.ID, ticket.Ticket); err == nil {
					t.Fatal("expired ticket attached")
				}
				return
			}
			if _, err = f.service.AttachLogSession(t.Context(), f.actor, httpID, f.cluster, ticket.Session.ID, ticket.Ticket); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "restart":
				b.pod.Status.ContainerStatuses[0].RestartCount++
			case "namespace":
				f.exec("UPDATE kubernetes_namespace_resources SET namespace_uid='replacement-uid' WHERE id=$1", f.namespace)
			case "cluster":
				f.exec("UPDATE kubernetes_clusters SET resource_version=resource_version+1 WHERE id=$1", f.cluster)
			case "logout":
				f.exec("UPDATE sessions SET revoked_at=now() WHERE id=$1", httpID)
			case "session-expiry":
				f.exec("UPDATE kubernetes_log_sessions SET expires_at=now()-interval '1 second' WHERE id=$1", ticket.Session.ID)
			case "closed":
				if _, err = f.service.CloseLogSession(t.Context(), f.actor, httpID, f.cluster, ticket.Session.ID); err != nil {
					t.Fatal(err)
				}
			}
			data, err := f.service.ReadLogSession(t.Context(), f.actor, httpID, f.cluster, ticket.Session.ID, "")
			if err == nil || data.Content != "" {
				t.Fatalf("%s continued: %+v %v", kind, data, err)
			}
		})
	}
}
