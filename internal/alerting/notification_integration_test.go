package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func notificationFixture(t *testing.T) (alertFixture, NotificationWorker, []db.AlertDelivery, []runtime.Task) {
	f := newAlertFixture(t)
	raw, _ := json.Marshal(ChannelSpec{Enabled: true, Provider: "wecom", CredentialID: uuid.New(), MentionIDs: []string{}})
	ch, err := f.s.Store.Queries.CreateAlertConfiguration(t.Context(), db.CreateAlertConfigurationParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, Kind: "channel", Name: "Protocol fixture", Spec: raw, CreatedBy: f.a.SubjectID})
	if err != nil {
		t.Fatal(err)
	}
	message, _ := json.Marshal(Message{Subject: "Fixture", Text: "CPU 95%", Test: true, Initiator: &f.a})
	rows := []db.AlertDelivery{}
	tasks := []runtime.Task{}
	for range 2 {
		d, err := f.s.Store.Queries.CreateAlertDelivery(t.Context(), db.CreateAlertDeliveryParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, ChannelID: ch.ID, ChannelVersion: 1, RouteVersion: 1, DedupKey: uuid.NewString(), GroupKey: "same-group", ResourceIds: []uuid.UUID{}, Payload: message, Status: "pending", AvailableAt: timestamp(time.Now().Add(-time.Second))})
		if err != nil {
			t.Fatal(err)
		}
		task := f.task(NotificationQueue)
		payload, _ := json.Marshal(deliveryTask{DeliveryID: d.ID})
		f.exec("UPDATE runtime_tasks SET payload=$2 WHERE id=$1", task.ID, payload)
		task.Payload = payload
		rows = append(rows, d)
		tasks = append(tasks, task)
	}
	return f, NotificationWorker{Service: f.s}, rows, tasks
}

func TestPostgresNotificationBatchMemberCannotInvalidateLiveSend(t *testing.T) {
	f, w, rows, tasks := notificationFixture(t)
	batch, err := w.claim(t.Context(), tasks[0], rows[0].ID)
	if err != nil || len(batch.Deliveries) != 2 {
		t.Fatal(batch, err)
	}
	other, err := w.claim(t.Context(), tasks[1], rows[1].ID)
	if err != nil || len(other.Deliveries) != 0 {
		t.Fatal("member task took live batch", other, err)
	}
	for _, d := range rows {
		current, err := f.s.Store.Queries.GetAlertDelivery(t.Context(), db.GetAlertDeliveryParams{EnterpriseID: d.EnterpriseID, ID: d.ID})
		if err != nil || current.Status != "sending" {
			t.Fatal(current, err)
		}
	}
	received := 0
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		received++
		if r.Header.Get("X-Argus-Notification-Batch") != batch.Persistent.ID.String() {
			t.Error("missing receipt correlation")
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"errcode":0}`))
	}))
	defer receiver.Close()
	if err = w.beginSend(t.Context(), tasks[0], batch); err != nil {
		t.Fatal(err)
	}
	transport := Transport{Client: receiver.Client(), ValidateEndpoint: func(context.Context, string) error { return nil }}
	result := transport.Send(t.Context(), batch.Spec, NotificationCredential{WebhookURL: receiver.URL}, batch.Message)
	if !result.Accepted {
		t.Fatal(result)
	}
	if err = w.finish(t.Context(), tasks[0], batch, result); err != nil {
		t.Fatal(err)
	}
	if received != 1 {
		t.Fatal("group transmitted more than once", received)
	}
	for _, d := range rows {
		current, err := f.s.Store.Queries.GetAlertDelivery(t.Context(), db.GetAlertDeliveryParams{EnterpriseID: d.EnterpriseID, ID: d.ID})
		if err != nil || current.Status != "accepted" || current.Attempt != 1 || current.BatchID.UUID != batch.Persistent.ID {
			t.Fatal(current, err)
		}
		attempts, err := f.s.Store.Queries.ListAlertDeliveryAttempts(t.Context(), db.ListAlertDeliveryAttemptsParams{EnterpriseID: d.EnterpriseID, DeliveryID: d.ID})
		if err != nil || len(attempts) != 1 || attempts[0].Status != "accepted" {
			t.Fatal(attempts, err)
		}
	}
}

func TestPostgresNotificationExhaustionDistinguishesBeforeAndAfterSend(t *testing.T) {
	for _, sent := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-send", true: "after-send"}[sent], func(t *testing.T) {
			f, w, rows, tasks := notificationFixture(t)
			batch, err := w.claim(t.Context(), tasks[0], rows[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if sent {
				if err = w.beginSend(t.Context(), tasks[0], batch); err != nil {
					t.Fatal(err)
				}
			}
			takeover := tasks[0]
			takeover.FenceToken++
			f.exec("UPDATE runtime_tasks SET fence_token=fence_token+1 WHERE id=$1", takeover.ID)
			if err = w.HandleExhausted(t.Context(), takeover, errors.New("worker terminated")); err != nil {
				t.Fatal(err)
			}
			if err = w.finish(t.Context(), tasks[0], batch, SendResult{Accepted: true}); !errors.Is(err, ErrConflict) {
				t.Fatal("stale fence returned acceptance", err)
			}
			for _, d := range rows {
				current, err := f.s.Store.Queries.GetAlertDelivery(t.Context(), db.GetAlertDeliveryParams{EnterpriseID: d.EnterpriseID, ID: d.ID})
				if err != nil {
					t.Fatal(err)
				}
				want := "pending"
				if sent {
					want = "unknown"
				}
				if current.Status != want {
					t.Fatal(current.Status, want)
				}
				attempts, err := f.s.Store.Queries.ListAlertDeliveryAttempts(t.Context(), db.ListAlertDeliveryAttemptsParams{EnterpriseID: d.EnterpriseID, DeliveryID: d.ID})
				if err != nil || len(attempts) != 1 {
					t.Fatal(attempts, err)
				}
			}
		})
	}
}
