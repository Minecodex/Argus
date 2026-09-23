package hostremoval

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"golang.org/x/crypto/ssh"
)

// Opt-in destructive checks use only newly-created disposable containers.
// The test owns and removes its PostgreSQL database and systemd target.
func TestRemovalIntegration(t *testing.T) {
	if os.Getenv("ARGUS_REMOVAL_INTEGRATION") != "1" {
		t.Skip("set ARGUS_REMOVAL_INTEGRATION=1 to run disposable PostgreSQL/systemd/SSH tests")
	}
	ctx := context.Background()
	pg := "argus-removal-pg-" + uuid.NewString()[:8]
	dockerTest(t, "run", "-d", "--label", "argus.io/test=host-removal", "--name", pg, "-e", "POSTGRES_PASSWORD=removal-test-only", "-p", "127.0.0.1::5432", "postgres:18.6-alpine")
	t.Cleanup(func() { dockerTest(t, "rm", "-f", "-v", pg) })
	address := strings.TrimSpace(dockerTest(t, "port", pg, "5432/tcp"))
	var store *postgres.Store
	var err error
	for i := 0; i < 40; i++ {
		store, err = postgres.Open(ctx, "postgres://postgres:removal-test-only@"+address+"/postgres?sslmode=disable")
		if err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err = postgres.RunMigrations(ctx, "postgres://postgres:removal-test-only@"+address+"/postgres?sslmode=disable", filepath.Join(root, "migrations/postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "argus-connector")
	build := exec.Command("go", "build", "-o", binary, "./cmd/argus-connector")
	build.Dir = root
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	service := Service{Store: store, TokenKey: bytes.Repeat([]byte{9}, 32), ExternalURL: "https://argus.test"}
	for _, scenario := range []struct {
		role   string
		manual bool
	}{{TargetManagedHost, false}, {TargetBastion, true}, {TargetBastion, false}} {
		role := scenario.role
		name := role + "_ssh"
		if scenario.manual {
			name = role + "_manual"
		}
		t.Run(name, func(t *testing.T) {
			operation, plan := removalFixture(t, store, role)
			parent := uuid.New()
			mustSQL(t, store, "INSERT INTO connectors(id,enterprise_id,role,name,host_id,bastion_scope_id,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at,status,connection_epoch) SELECT $1,enterprise_id,role,'Previous executor',host_id,bastion_scope_id,$2,device_fingerprint_hash,public_key_hash,certificate_expires_at,'online',1 FROM connectors WHERE id=$3", parent, parent.String(), operation.ConnectorID)
			staleCommand := "cmd_stale_" + uuid.NewString()
			mustSQL(t, store, "INSERT INTO connector_commands(id,command_id,enterprise_id,connector_id,connection_epoch,operation_ref,command_type,payload_schema_version,payload,payload_hash,idempotency_key,status,expires_at) VALUES($1,$2,$3,$4,1,$5,'host_connector_removal','argus.host_connector_removal/v1','{}',decode(repeat('00',32),'hex'),$2,'delivery_unknown',now()+interval '1 hour')", uuid.New(), staleCommand, operation.EnterpriseID, parent, operation.ID.String())
			// A valid reconnect and certificate rotation between commit and execution.
			mustSQL(t, store, "UPDATE connectors SET connection_epoch=6,version=2 WHERE id=$1", operation.ConnectorID)
			if err := service.validateOperationFence(ctx, store.Queries, operation); err != nil {
				t.Fatalf("normal reconnect invalidated removal: %v", err)
			}
			if role == TargetBastion {
				for _, state := range []string{"draining", "uninstalling", "removal_failed", "cleanup_unknown"} {
					mustSQL(t, store, "UPDATE bastion_scopes SET status=$2 WHERE id=$1", plan.BastionScopeID.UUID, state)
					rows, err := store.Queries.SetBastionRelayStatus(ctx, db.SetBastionRelayStatusParams{EnterpriseID: operation.EnterpriseID, ActiveConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true}, RelayStatus: "ready", RelayAddress: "10.10.0.2", RelayHttpsPort: 8445, RelayGatewayPort: 9445, RelayPortGeneration: 1})
					if err != nil || rows != 1 {
						t.Fatalf("heartbeat rejected in %s: rows=%d err=%v", state, rows, err)
					}
				}
				mustSQL(t, store, "UPDATE bastion_scopes SET status='draining' WHERE id=$1", plan.BastionScopeID.UUID)
			}
			bootstrap, receipt := fixtureTokens(t, service, operation)
			first, _, err := service.ClaimBootstrap(ctx, operation.ID, bootstrap)
			if err != nil {
				t.Fatal(err)
			}
			again, _, err := service.ClaimBootstrap(ctx, operation.ID, bootstrap)
			if err != nil || again != first {
				t.Fatalf("lost bootstrap response could not be replayed: %v", err)
			}
			mustSQL(t, store, "UPDATE connectors SET status='revoked' WHERE id=$1", operation.ConnectorID)
			if err = service.validateOperationFence(ctx, store.Queries, operation); err == nil {
				t.Fatal("revoked installation accepted")
			}
			mustSQL(t, store, "UPDATE connectors SET status='online' WHERE id=$1", operation.ConnectorID)
			mustSQL(t, store, "UPDATE hosts SET removal_generation=2 WHERE id=$1", operation.HostID)
			if err = service.validateOperationFence(ctx, store.Queries, operation); err == nil {
				t.Fatal("new removal generation accepted")
			}
			mustSQL(t, store, "UPDATE hosts SET removal_generation=1 WHERE id=$1", operation.HostID)
			target := newRemovalTarget(t, binary, plan)
			if scenario.manual {
				// Exercise the full generated command over strict HTTPS and the real
				// Helper, losing the first receipt response after the DB committed.
				runManualRemovalTarget(t, service, operation, plan, target, bootstrap, receipt)
			} else {
				if err = service.BeginRemote(ctx, mustOperation(t, store, operation)); err != nil {
					t.Fatal(err)
				}
				client := removalSSHClient(t, target)
				defer client.Close()
				wrong := plan
				wrong.ConnectorID = uuid.New()
				if _, _, _, err := ExecuteSSH(ctx, client, wrong); err == nil {
					t.Fatal("SSH accepted a different installed identity")
				}
				dockerTest(t, "exec", target, "systemctl", "is-active", "--quiet", "argus-connector")
				_, canonical, digest, err := ExecuteSSH(ctx, client, plan)
				if err != nil {
					t.Fatal(err)
				}
				_, replayed, _, err := ExecuteSSH(ctx, client, plan)
				if err != nil || !bytes.Equal(canonical, replayed) {
					t.Fatalf("SSH lost receipt replay: %v", err)
				}
				if err = service.FinalizeTrusted(ctx, operation, canonical, digest); err != nil {
					t.Fatal(err)
				}
			}
			current := mustOperation(t, store, operation)
			if current.Status != "succeeded" || current.Stage != "completed" {
				t.Fatalf("operation did not complete: %s/%s", current.Status, current.Stage)
			}
			stale, err := store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: staleCommand, ConnectorID: parent, ConnectionEpoch: 1})
			if err != nil || stale.Status != "failed" || stale.ErrorCode.String != "CONNECTOR_COMMAND_SUPERSEDED" {
				t.Fatalf("completed removal left its old Bastion attempt active: %s %v", stale.Status, err)
			}
			dockerTest(t, "exec", target, "sh", "-ec", "test ! -e /usr/local/bin/argus-connector; test ! -e /usr/local/bin/argus-otelcol; test ! -e /var/lib/argus-connector; ! id argus-connector >/dev/null 2>&1; systemctl is-active --quiet ssh")
		})
	}
}

func removalFixture(t *testing.T, store *postgres.Store, role string) (db.HostRemovalOperation, Plan) {
	t.Helper()
	ent, host, connector, action, op := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mustSQL(t, store, "INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Removal test',$2,'UTC')", ent, "removal-"+ent.String())
	if err := audit.InitializeChain(context.Background(), store.Queries, "enterprise", uuid.NullUUID{UUID: ent, Valid: true}); err != nil {
		t.Fatal(err)
	}
	var scope uuid.NullUUID
	hostRole, connectorRole := "managed_host", "host"
	if role == TargetBastion {
		hostRole, connectorRole = "bastion", "bastion"
		scope = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		mustSQL(t, store, "INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status,removal_generation) VALUES($1,$2,'Removal bastion','development',decode(repeat('00',32),'hex'),'command','draining',1)", scope.UUID, ent)
	}
	mustSQL(t, store, "INSERT INTO hosts(id,enterprise_id,name,platform,architecture,environment,labels_hash,role,control_path,bastion_scope_id,connector_id,status,removal_generation) VALUES($1,$2,'Removal host','linux','amd64','development',decode(repeat('00',32),'hex'),$3,'direct',$4,$5,'draining',1)", host, ent, hostRole, scope, connector)
	mustSQL(t, store, "INSERT INTO connectors(id,enterprise_id,role,name,host_id,bastion_scope_id,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at,status,connection_epoch) VALUES($1,$2,$3,'Removal connector',$4,$5,$6,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),now()+interval '1 day','online',1)", connector, ent, connectorRole, host, scope, connector.String())
	if scope.Valid {
		mustSQL(t, store, "UPDATE bastion_scopes SET connector_host_id=$2,active_connector_id=$3 WHERE id=$1", scope.UUID, host, connector)
	}
	mustSQL(t, store, "INSERT INTO pending_actions(id,action_ref,enterprise_id,creator_subject_id,authorization_version,action_type,title,summary,risk,preview,status,resource_type,resource_id,impact_hash,expires_at) VALUES($1,$2,$3,$4,1,'host.removal.uninstall','Removal','Removal','dangerous','{}','succeeded','host',$5,decode(repeat('00',32),'hex'),now()+interval '1 hour')", action, action.String(), ent, uuid.New(), host)
	plan := Plan{SchemaVersion: "argus.host_removal/v1", OperationID: uuid.NullUUID{UUID: op, Valid: true}, TargetType: role, HostID: host, TargetID: host, ConnectorID: connector, ConnectorVersion: 1, ConnectionEpoch: 1, RemovalGeneration: 1, TrustBundleEpoch: 1, TargetPlatform: "linux_amd64", ControlPath: "direct", DeliveryMethod: "manual", SSHPath: "none", Mode: "uninstall", BastionScopeID: scope}
	if scope.Valid {
		plan.TargetID = scope.UUID
		plan.RelayHTTPSPort = 8445
		plan.RelayGatewayPort = 9445
	}
	encoded, _ := canonicalPlan(plan)
	digest := sha256.Sum256(encoded)
	operation, err := store.Queries.CreateHostRemovalOperation(context.Background(), db.CreateHostRemovalOperationParams{ID: op, EnterpriseID: ent, PendingActionID: action, TargetType: role, HostID: host, BastionScopeID: scope, ConnectorID: connector, RemovalMode: "uninstall", DeliveryMethod: "manual", SshPath: "none", TargetPlatform: "linux_amd64", ControlPath: "direct", ResourceVersion: 1, ConnectorVersion: 1, ConnectionEpoch: 1, RemovalGeneration: 1, TrustBundleEpoch: 1, Plan: encoded, PlanHash: digest[:], Status: "awaiting_manual_execution", Stage: "awaiting_manual_execution", ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	return operation, plan
}

func fixtureTokens(t *testing.T, service Service, op db.HostRemovalOperation) (string, string) {
	t.Helper()
	bootstrap, _, _ := newToken()
	receipt, _, _ := newToken()
	for _, purpose := range []string{"bootstrap", "receipt"} {
		value := tokenEnvelope{Token: receipt}
		if purpose == "bootstrap" {
			value = tokenEnvelope{Token: bootstrap, ReceiptToken: receipt}
		}
		nonce, ciphertext, err := sealToken(service.TokenKey, op.EnterpriseID, op.ID, value)
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.Store.Queries.CreateHostRemovalToken(context.Background(), db.CreateHostRemovalTokenParams{ID: uuid.New(), OperationID: op.ID, EnterpriseID: op.EnterpriseID, Purpose: purpose, TokenHash: tokenDigest(value.Token), KeyVersion: tokenKeyVersion, Nonce: nonce, Ciphertext: ciphertext, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
	}
	return bootstrap, receipt
}

func mustSQL(t *testing.T, store *postgres.Store, query string, args ...any) {
	t.Helper()
	if _, err := store.Pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}
func mustOperation(t *testing.T, store *postgres.Store, op db.HostRemovalOperation) db.HostRemovalOperation {
	t.Helper()
	value, err := store.Queries.GetHostRemovalOperation(context.Background(), db.GetHostRemovalOperationParams{ID: op.ID, EnterpriseID: op.EnterpriseID})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func dockerTest(t *testing.T, args ...string) string {
	t.Helper()
	value, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v: %s", args, err, value)
	}
	return string(value)
}

func newRemovalTarget(t *testing.T, binary string, plan Plan) string {
	t.Helper()
	name := "argus-removal-target-" + uuid.NewString()[:8]
	image := os.Getenv("ARGUS_REMOVAL_SYSTEMD_IMAGE")
	if image == "" {
		image = "argus-systemd-host"
	}
	dockerTest(t, "run", "-d", "--privileged", "--label", "argus.io/test=host-removal", "--name", name, "--tmpfs", "/run", "--tmpfs", "/run/lock", "-p", "127.0.0.1::22", image)
	t.Cleanup(func() { dockerTest(t, "rm", "-f", "-v", name) })
	for i := 0; i < 40; i++ {
		if exec.Command("docker", "exec", name, "systemctl", "is-active", "--quiet", "ssh").Run() == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	dockerTest(t, "cp", binary, name+":/usr/local/bin/argus-connector")
	seed := `set -eu; useradd --system --shell /usr/sbin/nologin argus-connector; mkdir -p /var/lib/argus-connector /etc/argus-connector /var/lib/argus-otelcol /etc/argus-otelcol; chmod 755 /usr/local/bin/argus-connector; printf '{"connector_id":"%s"}' '` + plan.ConnectorID.String() + `' > /var/lib/argus-connector/identity.json; touch /usr/local/bin/argus-otelcol; for unit in argus-connector argus-connector-privileged argus-otelcol; do printf '[Service]\nExecStart=/bin/sleep infinity\n[Install]\nWantedBy=multi-user.target\n' > /etc/systemd/system/$unit.service; done; systemctl daemon-reload; systemctl enable --now argus-connector argus-connector-privileged argus-otelcol`
	if plan.TargetType == TargetBastion {
		dockerTest(t, "exec", name, "curl", "--version")
	}
	dockerTest(t, "exec", name, "sh", "-ec", seed)
	return name
}

func removalSSHClient(t *testing.T, target string) *ssh.Client {
	t.Helper()
	address := strings.TrimSpace(dockerTest(t, "port", target, "22/tcp"))
	public := dockerTest(t, "exec", target, "cat", "/etc/ssh/ssh_host_ed25519_key.pub")
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(public))
	if err != nil {
		t.Fatal(err)
	}
	password := uuid.NewString()
	cmd := exec.Command("docker", "exec", "-i", target, "chpasswd")
	cmd.Stdin = strings.NewReader("root:" + password + "\n")
	if err = cmd.Run(); err != nil {
		t.Fatal(err)
	}
	client, err := ssh.Dial("tcp", address, &ssh.ClientConfig{User: "root", Auth: []ssh.AuthMethod{ssh.Password(password)}, HostKeyAlgorithms: []string{ssh.KeyAlgoED25519}, HostKeyCallback: ssh.FixedHostKey(key), Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func runManualRemovalTarget(t *testing.T, service Service, op db.HostRemovalOperation, plan Plan, target, bootstrap, receipt string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "removal-e2e"}, DNSNames: []string{"host.docker.internal"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, _ := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	pair, _ := tls.X509KeyPair(ca, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	var failOnce atomic.Bool
	failOnce.Store(true)
	var dropBeforeReceipt atomic.Bool
	dropBeforeReceipt.Store(true)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			script, _, err := service.ClaimBootstrap(r.Context(), op.ID, r.Header.Get("X-Argus-Removal-Token"))
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			fmt.Fprint(w, script)
			return
		}
		var value Receipt
		if dropBeforeReceipt.Swap(false) {
			http.Error(w, "injected network failure before receipt", 503)
			return
		}
		if json.NewDecoder(r.Body).Decode(&value) != nil {
			http.Error(w, "invalid receipt", 400)
			return
		}
		if _, err := service.SubmitReceipt(r.Context(), r.Header.Get("X-Argus-Removal-Token"), value); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		if failOnce.Swap(false) {
			http.Error(w, "injected receipt response loss", 503)
			return
		}
		fmt.Fprint(w, `{}`)
	}))
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = server.Listener.Close()
	server.Listener = listener
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	service.ExternalURL = "https://host.docker.internal:" + port
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	_ = os.WriteFile(caPath, ca, 0600)
	dockerTest(t, "cp", caPath, target+":/var/lib/argus-connector/connector-ca.pem")
	receiptURL, _ := service.receiptURL()
	bootstrapURL, _ := service.bootstrapURL(op.ID)
	script := buildRemovalScript(plan, receiptURL, receipt)
	digest := sha256.Sum256([]byte(script))
	command, _, _ := buildDownloadCommand(plan, bootstrapURL, bootstrap, hex.EncodeToString(digest[:]))
	// Lose the first receipt before the database sees it; the whole original
	// command must still work using the preserved CA and local journal.
	if err = exec.Command("docker", "exec", target, "sh", "-c", command).Run(); err == nil {
		t.Fatal("injected network failure unexpectedly succeeded")
	}
	if mustOperation(t, service.Store, op).Status != "running" {
		t.Fatal("missing receipt was incorrectly marked complete")
	}
	// The second invocation commits but loses the HTTP response.
	output, runErr := exec.Command("docker", "exec", target, "sh", "-c", command).CombinedOutput()
	if runErr == nil {
		t.Fatal("lost receipt response unexpectedly succeeded")
	}
	if mustOperation(t, service.Store, op).Status != "succeeded" {
		t.Fatalf("cleanup receipt did not commit: %v: %s", runErr, output)
	}
	// Replay the local receipt over its still bounded consumed token.
	dockerTest(t, "exec", target, "sh", "-ec", `curl -fsS --cacert /var/lib/argus-uninstall/`+op.ID.String()+`/server-ca.pem --header `+shQuote("X-Argus-Removal-Token: "+receipt)+` --header 'Content-Type: application/json' --data-binary @/var/lib/argus-uninstall/`+op.ID.String()+`/receipt.json `+shQuote(receiptURL))
}
