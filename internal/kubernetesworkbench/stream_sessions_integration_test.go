package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"bytes"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	corev1 "k8s.io/api/core/v1"
)

func newStreamFixture(t *testing.T) (*mutationFixture, *sessionLogBackend, uuid.UUID, uuid.UUID, StreamInput) {
	t.Helper()
	f, b, httpID, log := newLogFixture(t)
	connector := uuid.New()
	f.exec("INSERT INTO connectors(id,enterprise_id,role,name,kubernetes_cluster_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at,capabilities) VALUES($1,$2,'kubernetes','stream-test',$3,$4,decode(repeat('ab',32),'hex'),decode(repeat('cd',32),'hex'),'online',1,now()+interval '1 hour',$5)", connector, f.actor.EnterpriseID, f.cluster, connector.String(), []string{kubernetesstream.SchemaVersion})
	f.exec("UPDATE kubernetes_clusters SET connector_id=$2 WHERE id=$1", f.cluster, connector)
	f.exec("INSERT INTO connector_sessions(connector_id,enterprise_id,gateway_instance_id,connection_epoch,capabilities,connected_at,last_heartbeat_at) VALUES($1,$2,'gateway',1,$3,now(),now())", connector, f.actor.EnterpriseID, []string{kubernetesstream.SchemaVersion})
	no := false
	b.pod.Spec = corev1.PodSpec{AutomountServiceAccountToken: &no, Containers: []corev1.Container{{Name: "main", Image: "alpine:3.21"}}}
	t.Cleanup(func() {
		_, e := f.service.Store.Pool.Exec(context.Background(), "DELETE FROM kubernetes_stream_sessions WHERE enterprise_id=$1", f.actor.EnterpriseID)
		if e != nil {
			t.Error(e)
		}
	})
	return f, b, httpID, connector, StreamInput{NamespaceID: log.NamespaceID, Type: "pod", Name: log.Pod, UID: log.PodUID, Container: log.Container, Mode: "exec", Shell: "sh", Reason: "Investigate the isolated test"}
}
func grantStream(f *mutationFixture, permissions ...string) {
	for _, permission := range permissions {
		f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,$3 FROM role_bindings WHERE enterprise_id=$1 AND subject_id=$2 ON CONFLICT DO NOTHING", f.actor.EnterpriseID, f.actor.SubjectID, permission)
	}
}

func TestPostgresKubernetesStreamTicketsAreBoundAndExecutionClaimIsAtomic(t *testing.T) {
	f, _, httpID, _, in := newStreamFixture(t)
	ctx := t.Context()
	if _, err := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatal("namespace grant implied exec", err)
	}
	grantStream(f, "kubernetes.exec")
	ticket, err := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in)
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.service.Store.Queries.GetKubernetesStreamSession(ctx, ticket.Session.ID)
	if err != nil || len(row.TicketHash) != 32 || string(row.TicketHash) == ticket.Ticket {
		t.Fatal("raw ticket persisted", err)
	}
	if _, err = f.service.GetStreamSession(ctx, f.actor, uuid.New(), f.cluster, row.ID); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatal("session moved across HTTP identity", err)
	}
	replacement, err := f.service.IssueStreamTicket(ctx, f.actor, httpID, f.cluster, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ClaimStream(ctx, row.ID, ticket.Ticket, "gateway-a"); err == nil {
		t.Fatal("replaced ticket accepted")
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := f.service.ClaimStream(ctx, row.ID, replacement.Ticket, "gateway-a"); e == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("ticket was not single-use", successes.Load())
	}
	successes.Store(0)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := f.service.ClaimStreamExecution(ctx, row.ID, row.Fence, "executor-b"); e == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("backend executed twice", successes.Load())
	}
	if err = f.service.ActivateStream(ctx, row.ID, row.Fence, "wrong-owner"); err == nil {
		t.Fatal("wrong owner activated")
	}
	if err = f.service.ActivateStream(ctx, row.ID, row.Fence, "gateway-a"); err != nil {
		t.Fatal(err)
	}
	if err = f.service.HeartbeatStream(ctx, row.ID, row.Fence, "gateway-a"); err != nil {
		t.Fatal(err)
	}
	closed, err := f.service.CloseStreamSession(ctx, f.actor, httpID, f.cluster, row.ID)
	if err != nil || closed.Status != "closing" {
		t.Fatal("active close was finalized before transport cleanup", err)
	}
	if _, err = f.service.CurrentStream(ctx, row.ID, row.Fence); err == nil {
		t.Fatal("closing stream accepted input")
	}
	if err = f.service.FinishStream(ctx, row.ID, row.Fence, "closed", "client_close", 123, 456); err != nil {
		t.Fatal(err)
	}
	final, err := f.service.GetStreamSession(ctx, f.actor, httpID, f.cluster, row.ID)
	if err != nil || final.Status != "closed" || final.BytesIn != 123 || final.BytesOut != 456 {
		t.Fatal("terminal metadata/counters lost", err, final)
	}
	if err = f.service.FinishStream(ctx, row.ID, row.Fence, "connection_lost", "duplicate_finish", 0, 0); err != nil {
		t.Fatal(err)
	}
	row, _ = f.service.Store.Queries.GetKubernetesStreamSession(ctx, row.ID)
	if row.Status != "closed" || row.BytesIn != 123 {
		t.Fatal("late cleanup overwrote final state")
	}
}
func TestPostgresKubernetesStreamRevocationRouteAndPrivilege(t *testing.T) {
	for _, scenario := range []string{"grant", "permission", "http_logout", "namespace_recreated", "connector_epoch", "capability", "cluster_version", "privilege", "service_account", "port_permission"} {
		t.Run(scenario, func(t *testing.T) {
			f, b, httpID, connector, in := newStreamFixture(t)
			ctx := t.Context()
			grantStream(f, "kubernetes.exec")
			if scenario == "privilege" {
				b.pod.Spec.HostNetwork = true
				if _, err := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in); err == nil {
					t.Fatal("namespace-only privileged terminal allowed")
				}
				return
			}
			if scenario == "service_account" {
				b.pod.Spec.AutomountServiceAccountToken = nil
				if _, err := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in); err == nil {
					t.Fatal("service-account token access implied by exec")
				}
				return
			}
			if scenario == "port_permission" {
				in.Mode, in.Shell, in.Port = "port_forward", "", 3306
				if _, err := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in); err == nil {
					t.Fatal("exec implied port forward")
				}
				return
			}
			ticket, err := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in)
			if err != nil {
				t.Fatal(err)
			}
			v, err := f.service.ClaimStream(ctx, ticket.Session.ID, ticket.Ticket, "gateway")
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "grant":
				f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_id=$2", f.actor.EnterpriseID, f.namespace)
			case "permission":
				f.exec("DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM role_bindings WHERE enterprise_id=$1) AND permission_id='kubernetes.exec'", f.actor.EnterpriseID)
			case "http_logout":
				f.exec("UPDATE sessions SET revoked_at=now() WHERE id=$1", httpID)
			case "namespace_recreated":
				f.exec("UPDATE kubernetes_namespace_resources SET namespace_uid='replacement' WHERE id=$1", f.namespace)
			case "connector_epoch":
				f.exec("UPDATE connectors SET connection_epoch=2 WHERE id=$1", connector)
			case "capability":
				f.exec("UPDATE connector_sessions SET capabilities='{}' WHERE connector_id=$1", connector)
			case "cluster_version":
				f.exec("UPDATE kubernetes_clusters SET resource_version=resource_version+1 WHERE id=$1", f.cluster)
			}
			if _, err = f.service.CurrentStream(ctx, v.Row.ID, v.Row.Fence); err == nil {
				t.Fatal("live stream survived revocation")
			}
		})
	}
}
func TestPostgresKubernetesStreamCredentialRotationAndDelivery(t *testing.T) {
	f, _, httpID, connector, in := newStreamFixture(t)
	ctx := t.Context()
	grantStream(f, "kubernetes.exec")
	f.service.Credentials.Idempotency = postgres.Idempotency{Key: bytes.Repeat([]byte{8}, 32)}
	record, err := f.service.Credentials.Create(ctx, f.actor.SubjectID.String(), f.actor.EnterpriseID, secret.SecretInput{Name: "stream kubeconfig", Type: "kubeconfig", Value: "test-sensitive-kubeconfig-not-an-actual-cluster"}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	credential, err := f.service.Credentials.CreateCredential(ctx, f.actor.SubjectID.String(), f.actor.EnterpriseID, secret.CredentialInput{Name: "stream credential", Protocol: "kubernetes", SecretID: record.Secret.ID}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE kubernetes_clusters SET connection_mode='direct',connector_id=NULL,credential_id=$2 WHERE id=$1", f.cluster, credential.ID)
	ticket, err := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in)
	if err != nil {
		t.Fatal(err)
	}
	v, err := f.service.ClaimStream(ctx, ticket.Session.ID, ticket.Ticket, "gateway")
	if err != nil {
		t.Fatal(err)
	}
	if !v.Row.CredentialLeaseID.Valid {
		t.Fatal("missing lease")
	}
	if err = ValidateStreamCredentialDelivery(ctx, f.service.Store.Queries, f.actor.EnterpriseID, v.Row.ID, connector, 1, v.Row.CredentialLeaseID.UUID); err == nil {
		t.Fatal("direct credential delivered to Connector")
	}
	if _, err = f.service.Credentials.FulfillLease(ctx, f.actor.EnterpriseID, v.Row.CredentialLeaseID.UUID, "connector_gateway", "wrong-gateway"); err == nil {
		t.Fatal("credential moved to foreign Gateway")
	}
	issued, err := f.service.Credentials.FulfillLease(ctx, f.actor.EnterpriseID, v.Row.CredentialLeaseID.UUID, "connector_gateway", "gateway")
	if err != nil || string(issued.Value) != "test-sensitive-kubeconfig-not-an-actual-cluster" {
		t.Fatal("recipient delivery", err)
	}
	clear(issued.Value)
	if err = ValidateStreamCredentialDelivery(ctx, f.service.Store.Queries, f.actor.EnterpriseID, v.Row.ID, uuid.New(), 1, v.Row.CredentialLeaseID.UUID); err == nil {
		t.Fatal("credential moved to foreign Connector")
	}
	var leaked bool
	row, _ := f.service.Store.Queries.GetKubernetesStreamSession(ctx, v.Row.ID)
	data, _ := json.Marshal(streamDTO(row, v.Binding))
	leaked = strings.Contains(string(data), "test-sensitive-kubeconfig-not-an-actual-cluster") || strings.Contains(string(row.Binding), "test-sensitive-kubeconfig-not-an-actual-cluster")
	if leaked {
		t.Fatal("secret exposed")
	}
	if _, err = f.service.Credentials.Rotate(ctx, f.actor.SubjectID.String(), f.actor.EnterpriseID, record.Secret.ID, secret.SecretInput{Value: "new-test-kubeconfig", ExpectedVersion: 1}, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.CurrentStream(ctx, v.Row.ID, v.Row.Fence); !errors.Is(err, kubernetesinventory.ErrIdentityChanged) {
		t.Fatal("credential rotation not detected", err)
	}
}

func TestPostgresKubernetesStreamCapacityClosingAndRetirement(t *testing.T) {
	f, _, httpID, _, in := newStreamFixture(t)
	ctx := t.Context()
	grantStream(f, "kubernetes.exec")
	var first StreamTicket
	for i := range 10 {
		ticket, e := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in)
		if e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			first = ticket
		}
	}
	if _, e := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in); !errors.Is(e, kubernetesinventory.ErrBudget) {
		t.Fatal("capacity exceeded", e)
	}
	v, e := f.service.ClaimStream(ctx, first.Session.ID, first.Ticket, "gateway")
	if e != nil {
		t.Fatal(e)
	}
	if e = f.service.ClaimStreamExecution(ctx, v.Row.ID, v.Row.Fence, "gateway"); e != nil {
		t.Fatal(e)
	}
	if e = f.service.ActivateStream(ctx, v.Row.ID, v.Row.Fence, "gateway"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.CloseStreamSession(ctx, f.actor, httpID, f.cluster, v.Row.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in); !errors.Is(e, kubernetesinventory.ErrBudget) {
		t.Fatal("closing session did not count toward capacity", e)
	}
	if e = f.service.FinishStream(ctx, v.Row.ID, v.Row.Fence, "closed", "client_close", 0, 0); e != nil {
		t.Fatal(e)
	}
	fresh, e := f.service.CreateStreamSession(ctx, f.actor, httpID, f.cluster, in)
	if e != nil {
		t.Fatal("capacity not released", e)
	}
	f.exec("UPDATE kubernetes_stream_sessions SET ticket_expires_at=now()-interval '1 minute' WHERE enterprise_id=$1 AND status='pending'", f.actor.EnterpriseID)
	if e = f.service.Store.Queries.RetireKubernetesStreamSessions(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.IssueStreamTicket(ctx, f.actor, httpID, f.cluster, fresh.Session.ID); e != nil {
		t.Fatal("expired ticket prevented replacement within pending session lifetime", e)
	}
	f.exec("UPDATE kubernetes_stream_sessions SET expires_at=now()-interval '1 minute' WHERE enterprise_id=$1 AND status='pending'", f.actor.EnterpriseID)
	if e = f.service.Store.Queries.RetireKubernetesStreamSessions(ctx); e != nil {
		t.Fatal(e)
	}
	var active int
	if e = f.service.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM kubernetes_stream_sessions WHERE enterprise_id=$1 AND status='pending'", f.actor.EnterpriseID).Scan(&active); e != nil || active != 0 {
		t.Fatal("pending tickets survived expiry", e)
	}
}
