package remoteexecution

import (
	"testing"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestUnconfirmedTerminationRemainsVisibleInPrivateActivity(t *testing.T) {
	f := newFixture(t)
	other := secondChat(f)
	v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("systemctl status private-service"))
	if err != nil {
		t.Fatal(err)
	}
	f.approve(v)
	f.exec("UPDATE remote_execution_requests SET status='paused_for_recovery' WHERE id=$1", v.ID)
	f.exec("UPDATE remote_execution_steps SET status='paused_for_recovery',result=$2 WHERE request_id=$1", v.ID, []byte(`{"terminated":false}`))
	run, err := f.store.Queries.GetRun(t.Context(), db.GetRunParams{ID: other.r, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	facts, err := f.service.ActivityFacts(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	if activities, ok := facts["activities"].([]map[string]any); !ok || len(activities) != 1 {
		t.Fatal("a potentially running operation disappeared from private activity")
	}
	f.exec("UPDATE remote_execution_steps SET result=$2 WHERE request_id=$1", v.ID, []byte(`{"terminated":true}`))
	facts, err = f.service.ActivityFacts(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	if activities := facts["activities"].([]map[string]any); len(activities) != 0 {
		t.Fatal("known terminated operation still reported active")
	}
}
