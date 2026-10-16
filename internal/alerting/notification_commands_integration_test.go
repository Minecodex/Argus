package alerting

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

func TestPostgresNotificationCommandReturnsItsNewDelivery(t *testing.T) {
	f, _, rows, _ := notificationFixture(t)
	preview, err := f.s.PreviewCommand(t.Context(), f.a, Command{Operation: "notification.test", ID: rows[0].ChannelID, ExpectedVersion: 1}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.commit(preview)
	if err != nil || result.ResourceType != "alert_delivery" || result.ResourceID == rows[0].ChannelID {
		t.Fatal("test command returned channel instead of delivery", result, err)
	}
	original, err := f.s.Store.Queries.GetAlertDelivery(t.Context(), db.GetAlertDeliveryParams{EnterpriseID: f.a.EnterpriseID, ID: result.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE alert_deliveries SET status='unknown' WHERE id=$1", original.ID)
	preview, err = f.s.PreviewCommand(t.Context(), f.a, Command{Operation: "notification.resend", ID: original.ID, ExpectedVersion: original.Version}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	result, err = f.commit(preview)
	if err != nil || result.ResourceType != "alert_delivery" || result.ResourceID == original.ID {
		t.Fatal("resend returned old delivery", result, err)
	}
	resend, err := f.s.Store.Queries.GetAlertDelivery(t.Context(), db.GetAlertDeliveryParams{EnterpriseID: f.a.EnterpriseID, ID: result.ResourceID})
	if err != nil || resend.ResendOfID.UUID != original.ID {
		t.Fatal("new delivery has no original link", resend, err)
	}
}
