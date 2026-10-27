package telemetry

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/twmb/franz-go/pkg/kgo"
	logscollector "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestClickHouseStructuredFileLogBodyRemainsSearchableWithoutLosingFields(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	enterprise, app, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), enterprise); err != nil {
			t.Error(err)
		}
	})
	marker := "plan8-json-message-" + uuid.NewString()
	at := time.Now().UTC().Truncate(time.Millisecond)
	request := &logscollector.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
		{Key: "argus.source.id", Value: stringValue(source.String())},
		{Key: "argus.source.revision", Value: stringValue("1")},
		{Key: "argus.source.type", Value: stringValue("filelog")},
		{Key: "service.name", Value: stringValue("file-api")},
	}}, ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{TimeUnixNano: uint64(at.UnixNano()), Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{
		{Key: "message", Value: stringValue(marker)},
		{Key: "level", Value: stringValue("info")},
	}}}}}}}}}}}
	writer := Writer{ClickHouse: conn, Router: router}
	identity := TrustedIdentity{EnterpriseID: enterprise, CollectorID: uuid.New(), ResourceID: app, ResourceType: "application"}
	if _, err := writer.writeLogs(ctx, &kgo.Record{Topic: "otlp-logs", Offset: 1}, identity, request, time.Hour); err != nil {
		t.Fatal(err)
	}
	result, err := kql.Execute(ctx, conn, router, kql.Request{Expression: "body : " + strconv.Quote(marker), Start: at.Add(-time.Minute), End: at.Add(time.Minute), Scope: kql.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{app}, SourceKeys: []string{source.String() + ":1"}}, Budget: kql.Budget{MaxRows: 100, Timeout: 10 * time.Second}})
	if err != nil || len(result.Data) != 1 {
		t.Fatalf("structured message not searchable: rows=%d err=%v", len(result.Data), err)
	}
	row := result.Data[0]
	body, ok := row["body"].(string)
	if !ok || body == marker {
		t.Fatal("structured body was replaced by only its message")
	}
	var object map[string]any
	if json.Unmarshal([]byte(body), &object) != nil || object["message"] != marker || object["level"] != "info" {
		t.Fatal("structured log metadata lost", body)
	}
	if row["resource_id"] != app {
		t.Fatal("structured log escaped its application")
	}
}
