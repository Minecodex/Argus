package connector

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

func TestReconnectRequiresCurrentCredentialExactTargetKeyAndCallback(t *testing.T) {
	actor, credentialID := uuid.New(), uuid.New()
	host := db.Host{Platform: "linux", PinnedHostKey: "SHA256:fixed"}
	tunnel := db.ConnectorControlTunnel{TargetAddress: "fixed-host", TargetPort: 2222, TargetUsername: "root"}
	credential := db.Credential{ID: credentialID, Version: 2}
	plan := []byte(`{"address":"fixed-host","port":2222,"platform":"linux","username":"root","ssh_path":"direct_executor","onboarding":{"control_path":"executor_tunnel"}}`)
	hash := sha256.Sum256(plan)
	result, _ := json.Marshal(resource.ConnectionTestResult{HostKeyFingerprint: host.PinnedHostKey, CallbackVerified: true, CallbackControlPath: "executor_tunnel"})
	test := db.ConnectionTest{Status: "succeeded", TargetType: "host", Path: "direct", CreatedBy: actor, CredentialID: uuid.NullUUID{UUID: credentialID, Valid: true}, CredentialVersion: pgtype.Int8{Int64: 2, Valid: true}, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true}, RequestPlan: plan, RequestHash: hash[:], Result: result}
	if !validReconnectTest(test, host, tunnel, credential, actor) {
		t.Fatal("matching verified reconnect denied")
	}
	for _, change := range []func(*db.ConnectionTest){
		func(x *db.ConnectionTest) { x.CreatedBy = uuid.New() },
		func(x *db.ConnectionTest) { x.CredentialVersion.Int64 = 1 },
		func(x *db.ConnectionTest) { x.ExpiresAt.Time = time.Now().Add(-time.Minute) },
		func(x *db.ConnectionTest) {
			x.Result = []byte(`{"host_key_fingerprint":"SHA256:changed","callback_verified":true,"callback_control_path":"executor_tunnel"}`)
		},
		func(x *db.ConnectionTest) {
			x.Result = []byte(`{"host_key_fingerprint":"SHA256:fixed","callback_verified":false}`)
		},
		func(x *db.ConnectionTest) { x.RequestHash = []byte("tampered") },
	} {
		candidate := test
		change(&candidate)
		if validReconnectTest(candidate, host, tunnel, credential, actor) {
			t.Fatal("stale or mismatched reconnect proof accepted")
		}
	}
}
