package alerting

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

func TestSourceKeysUseTheExistingQueryRevisionContract(t *testing.T) {
	id := uuid.New()
	values := sourceKeys([]db.AlertMetricSourcesRow{{ID: id, SourceKey: "otlp/metrics", ConfigRevision: 7}})
	if len(values) != 1 || values[0] != id.String()+":7" {
		t.Fatal("raw collector source key bypassed the Query RPC contract", values)
	}
}
