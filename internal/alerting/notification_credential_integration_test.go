package alerting

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Broker/database failures happen before the sending boundary. Their SQLSTATE
// must reach the runtime so a recovered batch can obtain a new fenced lease.
func TestPostgresNotificationCredentialSQLFailureRecoversBeforeSend(t *testing.T) {
	for _, stage := range []string{"issue", "consume"} {
		for _, code := range []string{"40001", "40P01"} {
			t.Run(stage+"/"+code, func(t *testing.T) {
				f, worker, deliveries, tasks := notificationFixture(t)
				received := 0
				receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					received++
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"errcode":0}`))
				}))
				defer receiver.Close()
				keyfile := filepath.Join(t.TempDir(), "test-keyring.json")
				keydata, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))}})
				if err := os.WriteFile(keyfile, keydata, 0600); err != nil {
					t.Fatal(err)
				}
				keyring, err := secret.LoadKeyring(keyfile)
				if err != nil {
					t.Fatal(err)
				}
				f.s.Secrets = secret.Service{Store: f.s.Store, Keyring: keyring, Idempotency: f.s.Actions.Idempotency}
				value, _ := json.Marshal(NotificationCredential{WebhookURL: receiver.URL})
				stored, err := f.s.Secrets.Create(t.Context(), f.a.SubjectID.String(), f.a.EnterpriseID, secret.SecretInput{Name: "Notification retry", Type: "notification", Value: string(value)}, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				credential, err := f.s.Secrets.CreateCredential(t.Context(), f.a.SubjectID.String(), f.a.EnterpriseID, secret.CredentialInput{Name: "Notification retry", Protocol: "notification", SecretID: stored.Secret.ID}, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				spec, _ := json.Marshal(ChannelSpec{Enabled: true, Provider: "wecom", CredentialID: credential.ID, MentionIDs: []string{}})
				f.exec("UPDATE alert_configurations SET spec=$2 WHERE id=$1", deliveries[0].ChannelID, spec)
				worker.Service = f.s
				worker.Transport = Transport{Client: receiver.Client(), ValidateEndpoint: func(context.Context, string) error { return nil }}
				name := "p6_" + strings.ReplaceAll(uuid.NewString(), "-", "")
				event, condition := "INSERT", fmt.Sprintf("NEW.credential_id='%s'::uuid", credential.ID)
				if stage == "consume" {
					event, condition = "UPDATE", condition+" AND OLD.status='active' AND NEW.status='consumed'"
				}
				f.exec(fmt.Sprintf("CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF %s THEN RAISE EXCEPTION 'Plan6 injected broker SQL failure' USING ERRCODE='%s'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE %s ON credential_leases FOR EACH ROW EXECUTE FUNCTION %s();", name, condition, code, name, event, name))
				t.Cleanup(func() {
					_, _ = f.s.Store.Pool.Exec(context.Background(), fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON credential_leases; DROP FUNCTION IF EXISTS %s();", name, name))
				})
				err = worker.Handle(t.Context(), tasks[0])
				var pg *pgconn.PgError
				if !errors.As(err, &pg) || pg.Code != code || !strings.Contains(err.Error(), "credential "+stage) || received != 0 {
					t.Fatalf("credential failure lost its stage/SQLSTATE or sent: %v, receipts=%d", err, received)
				}
				current, err := f.s.Store.Queries.GetAlertDelivery(t.Context(), db.GetAlertDeliveryParams{EnterpriseID: f.a.EnterpriseID, ID: deliveries[0].ID})
				if err != nil || current.Status != "sending" {
					t.Fatal("database failure became a terminal delivery", current, err)
				}
				batch, err := f.s.Store.Queries.GetAlertDeliveryBatch(t.Context(), db.GetAlertDeliveryBatchParams{EnterpriseID: f.a.EnterpriseID, ID: current.BatchID.UUID})
				if err != nil || batch.Status != "prepared" || batch.SendStartedAt.Valid {
					t.Fatal("database failure crossed the sending boundary", batch, err)
				}
				f.exec(fmt.Sprintf("DROP TRIGGER %s ON credential_leases; DROP FUNCTION %s();", name, name))
				takeover := tasks[0]
				takeover.FenceToken++
				f.exec("UPDATE runtime_tasks SET fence_token=$2 WHERE id=$1", takeover.ID, takeover.FenceToken)
				if err = worker.HandleExhausted(t.Context(), takeover, errors.New("fixture lost worker")); err != nil {
					t.Fatal(err)
				}
				f.exec("UPDATE alert_channel_rate_limits SET not_before=now()-interval '1 second' WHERE channel_id=$1", deliveries[0].ChannelID)
				next := f.task(NotificationQueue)
				payload, _ := json.Marshal(deliveryTask{DeliveryID: deliveries[0].ID})
				f.exec("UPDATE runtime_tasks SET payload=$2 WHERE id=$1", next.ID, payload)
				next.Payload = payload
				if err = worker.Handle(t.Context(), next); err != nil || received != 1 {
					t.Fatal("recovered batch did not send once", err, received)
				}
				for _, delivery := range deliveries {
					current, err := f.s.Store.Queries.GetAlertDelivery(t.Context(), db.GetAlertDeliveryParams{EnterpriseID: f.a.EnterpriseID, ID: delivery.ID})
					if err != nil || current.Status != "accepted" || current.Attempt != 2 {
						t.Fatal("recovered delivery not accepted", current, err)
					}
				}
			})
		}
	}
}
