package kubernetesstreamgateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/kubernetesstreamclient"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type gatewayInventory struct{ pod *corev1.Pod }

func (b gatewayInventory) ListInventory(context.Context, db.KubernetesCluster, kubernetesinventory.Query) (kubernetesinventory.Page, error) {
	return kubernetesinventory.Page{}, kubernetesinventory.ErrUnsupported
}
func (b gatewayInventory) GetInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	if q.Namespace != "orders" || q.NamespaceUID != "orders-uid" || q.Name != b.pod.Name || q.UID != string(b.pod.UID) {
		return kubernetesinventory.Object{}, kubernetesinventory.ErrIdentityChanged
	}
	raw, _ := json.Marshal(b.pod)
	d, _ := kubernetesinventory.Lookup("pod")
	return kubernetesinventory.Object{Definition: d, Name: b.pod.Name, Namespace: "orders", NamespaceUID: "orders-uid", UID: string(b.pod.UID), ResourceVersion: "1", Data: raw}, nil
}

type gatewayFixture struct {
	service                  kubernetesworkbench.Service
	actor                    kubernetesworkbench.Actor
	http, cluster, namespace uuid.UUID
	factory                  *echoFactory
	server                   *httptest.Server
	exec                     func(string, ...any)
}

func newGatewayFixture(t *testing.T) *gatewayFixture {
	t.Helper()
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	if e := postgres.RunMigrations(t.Context(), address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); e != nil {
		t.Fatal(e)
	}
	store, e := postgres.Open(t.Context(), address)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(store.Close)
	f := &gatewayFixture{actor: kubernetesworkbench.Actor{EnterpriseID: uuid.New(), SubjectID: uuid.New(), SubjectType: "user", AuthorizationVersion: 1}, http: uuid.New(), cluster: uuid.New(), namespace: uuid.New()}
	f.exec = func(query string, args ...any) {
		t.Helper()
		if _, e := store.Pool.Exec(t.Context(), query, args...); e != nil {
			t.Fatal(e)
		}
	}
	department, role, connector := uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan8 stream Gateway',$2,'UTC')", f.actor.EnterpriseID, f.actor.EnterpriseID.String())
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'operators')", department, f.actor.EnterpriseID)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'operator')", f.actor.SubjectID, f.actor.EnterpriseID, department, f.actor.SubjectID.String())
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'stream operator')", role, f.actor.EnterpriseID)
	for id, description := range authorization.PermissionRegistry {
		f.exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", id, description, authorization.PermissionRegistryVersion)
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.read'),($1,'kubernetes.exec'),($1,'kubernetes.port_forward')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, role)
	f.exec("INSERT INTO sessions(id,token_hash,csrf_hash,audience,user_id,enterprise_id,department_id,authorization_version,locale,idle_expires_at,absolute_expires_at,last_seen_at) VALUES($1,decode(repeat(replace($1::uuid::text,'-',''),2),'hex'),decode(repeat('ef',32),'hex'),'enterprise',$2,$3,$4,1,'zh-CN',now()+interval '1 hour',now()+interval '1 hour',now())", f.http, f.actor.SubjectID, f.actor.EnterpriseID, department)
	f.exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'stream cluster','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", f.cluster, f.actor.EnterpriseID)
	f.exec("INSERT INTO connectors(id,enterprise_id,role,name,kubernetes_cluster_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at,capabilities) VALUES($1,$2,'kubernetes','stream-connector',$3,$4,decode(repeat('ab',32),'hex'),decode(repeat('cd',32),'hex'),'online',1,now()+interval '1 hour',$5)", connector, f.actor.EnterpriseID, f.cluster, connector.String(), []string{kubernetesstream.SchemaVersion})
	f.exec("UPDATE kubernetes_clusters SET connector_id=$2 WHERE id=$1", f.cluster, connector)
	f.exec("INSERT INTO connector_sessions(connector_id,enterprise_id,gateway_instance_id,connection_epoch,capabilities,connected_at,last_heartbeat_at) VALUES($1,$2,'gateway-a',1,$3,now(),now())", connector, f.actor.EnterpriseID, []string{kubernetesstream.SchemaVersion})
	if _, e = store.Queries.UpsertKubernetesNamespaceResource(t.Context(), db.UpsertKubernetesNamespaceResourceParams{ID: f.namespace, EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, Name: "orders", NamespaceUid: "orders-uid"}); e != nil {
		t.Fatal(e)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, f.namespace)
	if e = audit.InitializeChain(t.Context(), store.Queries, "enterprise", uuid.NullUUID{UUID: f.actor.EnterpriseID, Valid: true}); e != nil {
		t.Fatal(e)
	}
	no := false
	pod := &corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}, ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "orders", UID: "pod-uid", ResourceVersion: "1"}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, Containers: []corev1.Container{{Name: "main"}}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "main", ContainerID: "containerd://pinned"}}}}
	f.service = kubernetesworkbench.Service{Store: store, Backend: gatewayInventory{pod: pod}}
	f.factory = &echoFactory{service: f.service}
	f.server = httptest.NewServer(WebSocketGateway{Service: f.service, InstanceID: "gateway-a", Backends: f.factory})
	t.Cleanup(f.server.Close)
	t.Cleanup(func() {
		_, e := store.Pool.Exec(context.Background(), "DELETE FROM kubernetes_stream_sessions WHERE enterprise_id=$1", f.actor.EnterpriseID)
		if e != nil {
			t.Error(e)
		}
		_, _ = store.Pool.Exec(context.Background(), "DELETE FROM sessions WHERE id=$1", f.http)
	})
	return f
}
func (f *gatewayFixture) ticket(t *testing.T, mode string) kubernetesworkbench.StreamTicket {
	t.Helper()
	input := kubernetesworkbench.StreamInput{NamespaceID: f.namespace, Type: "pod", Name: "api", UID: "pod-uid", Mode: mode, Reason: "isolated transport test"}
	if mode == "exec" {
		input.Shell = "sh"
		input.Container = "main"
	} else {
		input.Port = 3306
	}
	v, e := f.service.CreateStreamSession(t.Context(), f.actor, f.http, f.cluster, input)
	if e != nil {
		t.Fatal(e)
	}
	v.WebsocketURL = "ws" + strings.TrimPrefix(f.server.URL, "http") + "/v1/kubernetes/sessions/" + v.Session.ID.String()
	return v
}

type echoFactory struct {
	service kubernetesworkbench.Service
	opens   atomic.Int32
}

func (f *echoFactory) Open(ctx context.Context, v kubernetesworkbench.StreamConnection, _, _ int) (interactive.Session, error) {
	if e := f.service.ClaimStreamExecution(ctx, v.Row.ID, v.Row.Fence, "gateway-a"); e != nil {
		return nil, e
	}
	f.opens.Add(1)
	return &echoStream{ctx: ctx, mode: v.Row.Mode, events: make(chan interactive.Frame, 32), done: make(chan struct{})}, nil
}

type echoStream struct {
	ctx    context.Context
	mode   string
	events chan interactive.Frame
	done   chan struct{}
	once   sync.Once
}

func (s *echoStream) Send(ctx context.Context, in interactive.Input) error {
	if in.Type == "resize" {
		return nil
	}
	data := []byte(in.Data)
	if s.mode == "port_forward" {
		kind, id, payload, e := kubernetesstream.ParsePacket(data)
		if e != nil {
			return e
		}
		switch kind {
		case kubernetesstream.OpenChannel:
			return nil
		case kubernetesstream.ChannelData:
			data = kubernetesstream.Packet(kind, id, payload)
		case kubernetesstream.ChannelEOF:
			data = kubernetesstream.Packet(kubernetesstream.CloseChannel, id, nil)
		case kubernetesstream.CloseChannel:
			return nil
		}
	}
	select {
	case s.events <- interactive.Frame{Type: "output", Data: data}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return io.EOF
	}
}
func (s *echoStream) Receive(ctx context.Context) (interactive.Frame, error) {
	select {
	case f := <-s.events:
		return f, nil
	case <-ctx.Done():
		return interactive.Frame{}, ctx.Err()
	case <-s.done:
		return interactive.Frame{}, io.EOF
	}
}
func (s *echoStream) Close(context.Context, string) error {
	s.once.Do(func() { close(s.done) })
	return nil
}

func TestPostgresKubernetesWebSocketTicketsRevocationAndAudit(t *testing.T) {
	f := newGatewayFixture(t)
	ticket := f.ticket(t, "exec")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	dial := func(v kubernetesworkbench.StreamTicket) (*websocket.Conn, ServerFrame) {
		t.Helper()
		ws, _, e := websocket.Dial(ctx, v.WebsocketURL, nil)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = ws.CloseNow() })
		hello, _ := json.Marshal(interactive.Input{Protocol: kubernetesstream.WebSocketProtocol, Type: "client_hello", Sequence: 1, Ticket: v.Ticket, Cols: 100, Rows: 30})
		if e = ws.Write(ctx, websocket.MessageText, hello); e != nil {
			t.Fatal(e)
		}
		_, raw, e := ws.Read(ctx)
		if e != nil {
			t.Fatal(e)
		}
		var ready ServerFrame
		if json.Unmarshal(raw, &ready) != nil || ready.Type != "server_ready" || ready.Sequence != 1 {
			t.Fatal("ready protocol")
		}
		return ws, ready
	}
	ws, ready := dial(ticket)
	canary := "manual-terminal-input-must-not-be-audited"
	input, _ := json.Marshal(interactive.Input{Protocol: kubernetesstream.WebSocketProtocol, Type: "input", Sequence: 2, Nonce: ready.Nonce, Data: canary})
	if e := ws.Write(ctx, websocket.MessageText, input); e != nil {
		t.Fatal(e)
	}
	_, raw, e := ws.Read(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var output ServerFrame
	_ = json.Unmarshal(raw, &output)
	decoded, _ := base64.StdEncoding.DecodeString(output.Data)
	if output.Type != "output" || string(decoded) != canary {
		t.Fatal("terminal bytes changed")
	}
	replay, _, e := websocket.Dial(ctx, ticket.WebsocketURL, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer replay.CloseNow()
	hello, _ := json.Marshal(interactive.Input{Protocol: kubernetesstream.WebSocketProtocol, Type: "client_hello", Sequence: 1, Ticket: ticket.Ticket, Cols: 100, Rows: 30})
	_ = replay.Write(ctx, websocket.MessageText, hello)
	if _, _, e = replay.Read(ctx); e == nil {
		t.Fatal("ticket replay started another terminal")
	}
	if f.factory.opens.Load() != 1 {
		t.Fatal("duplicate execution")
	}
	f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_id=$2", f.actor.EnterpriseID, f.namespace)
	_, raw, e = ws.Read(ctx)
	if e != nil {
		t.Fatal("missing revocation state", e)
	}
	var end ServerFrame
	_ = json.Unmarshal(raw, &end)
	if end.Type != "session_state" || end.Status != "invalidated" {
		t.Fatal("live revocation failed", string(raw))
	}
	for deadline := time.Now().Add(5 * time.Second); ; {
		row, e := f.service.Store.Queries.GetKubernetesStreamSession(ctx, ticket.Session.ID)
		if e == nil && row.Status == "invalidated" {
			if row.BytesIn != int64(len(canary)) || row.BytesOut != int64(len(canary)) {
				t.Fatal("audit counters", row.BytesIn, row.BytesOut)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("final state missing", e)
		}
		time.Sleep(20 * time.Millisecond)
	}
	rows, e := f.service.Store.Pool.Query(ctx, "SELECT details::text FROM audit_events WHERE enterprise_id=$1 AND resource_id=$2", f.actor.EnterpriseID, ticket.Session.ID.String())
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var value string
		if rows.Scan(&value) != nil {
			t.Fatal("audit scan")
		}
		if strings.Contains(value, canary) || strings.Contains(value, ticket.Ticket) {
			t.Fatal("sensitive terminal input or ticket audited")
		}
		if !strings.Contains(value, "pod-uid") || !strings.Contains(value, "orders-uid") || !strings.Contains(value, "isolated transport test") {
			t.Fatal("audit lost frozen target or request reason")
		}
		count++
	}
	if count < 3 {
		t.Fatal("lifecycle audit missing", count)
	}
}
func TestPostgresKubernetesForwardClientMultipleTCPConnectionsAndExplicitClose(t *testing.T) {
	f := newGatewayFixture(t)
	ticket := f.ticket(t, "port_forward")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	listener, e := kubernetesstreamclient.Listen("127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- kubernetesstreamclient.Forward(ctx, listener, kubernetesstreamclient.Options{URL: ticket.WebsocketURL, Ticket: ticket.Ticket, Ready: func() { close(ready) }})
	}()
	select {
	case <-ready:
	case e := <-done:
		t.Fatal(e)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, e := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
			if e != nil {
				failures <- e
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			payload := bytes.Repeat([]byte{byte(i), 0, 255, 128, 'x'}, 15000)
			if _, e = conn.Write(payload); e != nil {
				failures <- e
				return
			}
			if e = conn.(*net.TCPConn).CloseWrite(); e != nil {
				failures <- e
				return
			}
			got, e := io.ReadAll(conn)
			if e != nil {
				failures <- e
				return
			}
			if !bytes.Equal(got, payload) {
				failures <- io.ErrUnexpectedEOF
			}
		}()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Error(e)
	}
	if _, e = f.service.CloseStreamSession(ctx, f.actor, f.http, f.cluster, ticket.Session.ID); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal("explicit forwarding close", e)
		}
	case <-ctx.Done():
		t.Fatal("forward close did not end client")
	}
	if _, e = net.DialTimeout("tcp", listener.Addr().String(), time.Second); e == nil {
		t.Fatal("local listener survived session close")
	}
}
