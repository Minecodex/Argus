package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
)

func TestClickHouseProjectionSelectionPreservesSourcePriority(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	tenant, target, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), tenant) })
	tables, _ := router.Tables(tenant)
	at := time.Now().UTC().Truncate(time.Minute)
	// Two physical generations for each of 400 logical series. UUID order and
	// Kafka offset deliberately do not define the winning source generation.
	metadata := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,labels,labels_hash,resource_attributes,metric_type,expires_at,source_id,source_revision,source_type)
SELECT toUUID(?),reinterpretAsUUID(MD5(concat('wanted-',toString(number%%400),'-',toString(intDiv(number,400))))),
'projection_priority_metric',map('device',toString(number%%400)),cityHash64(number%%400),
map('argus.assignment.epoch',toString(intDiv(number,400)+1)),'gauge',now64(3)+INTERVAL 1 HOUR,toUUID(?),intDiv(number,400)+1,'kubeletstats' FROM numbers(800)`, tables.MetricSeries)
	if err := conn.Exec(ctx, metadata, target.String(), source.String()); err != nil {
		t.Fatal(err)
	}
	// Put noise and both generations in the same part, with interleaved UUIDs.
	// A join that blocks the metric/time projection touches almost every granule.
	samples := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,timestamp,float_value,sample_type,ingest_key,kafka_offset,expires_at,source_id,source_revision,source_type)
SELECT resource_id,series_id,metric_name,fromUnixTimestamp64Milli(?),toFloat64(source_revision),'gauge',toString(series_id),100-source_revision,expires_at,source_id,source_revision,source_type FROM %s
UNION ALL SELECT toUUID(?),reinterpretAsUUID(MD5(concat('noise-',toString(number%%10000)))),concat('noise_',toString(number%%1000)),fromUnixTimestamp64Milli(?),1,'gauge',toString(number),0,now64(3)+INTERVAL 1 HOUR,toUUID(?),1,'kubeletstats' FROM numbers(800000)`, tables.MetricSamples, tables.MetricSeries)
	if err := conn.Exec(ctx, samples, at.UnixMilli(), target.String(), at.UnixMilli(), source.String()); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(ctx, "OPTIMIZE TABLE "+tables.MetricSamples+" FINAL"); err != nil {
		t.Fatal(err)
	}
	request := promengine.Request{Expression: "sum(projection_priority_metric)", Instant: true, Start: at.Add(-time.Minute), End: at,
		Scope: promengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{target},
			SourceKeys: []string{source.String() + ":1", source.String() + ":2"}},
		MaxSeries: 1000, MaxSamples: 2000, MaxScanBytes: 8 << 20, Timeout: 20 * time.Second}
	result, err := promengine.NewEngine(conn, router, nil).Execute(ctx, request)
	if err != nil {
		t.Fatalf("projection-readable selection exceeded a small budget: %v", err)
	}
	vector, ok := result.Value.(promql.Vector)
	if !ok || len(vector) != 1 || vector[0].F != 800 {
		t.Fatal("source precedence changed during projection selection", result.Value)
	}
	if result.ScannedRows >= 40000 || result.ScannedBytes >= 8<<20 {
		t.Fatalf("sample selection scanned unrelated UUID granules: rows=%d bytes=%d", result.ScannedRows, result.ScannedBytes)
	}
	t.Logf("400 logical series, two source generations, 800k noise: rows=%d bytes=%d", result.ScannedRows, result.ScannedBytes)
}
