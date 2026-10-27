package connector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestPostgresCollectorCommandExpirationProjectsFailedAndUnknownOutcomes(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "collector_timeout", filepath.Join("..", "..", "migrations", "postgresql"))
	ctx := t.Context()
	enterprise, creator := uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone)VALUES($1,'Owned command expiration',$2,'UTC')", enterprise, enterprise.String())
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterprise, Valid: true}); err != nil {
		t.Fatal(err)
	}
	type scenario struct {
		name, status, commandExpected, collectorExpected, applyExpected, code, initialCollector, initialApply, routeExpected string
		desired, effective                                                                                                   int64
		host, connector, collector, collection                                                                               uuid.UUID
	}
	scenarios := []scenario{
		{name: "never-delivered", status: "queued", commandExpected: "expired", collectorExpected: "degraded", applyExpected: "failed", code: "CONNECTOR_COMMAND_EXPIRED", routeExpected: "active"},
		{name: "sent-unacknowledged", status: "dispatched", commandExpected: "result_unknown", collectorExpected: "result_unknown", applyExpected: "unknown", code: "CONNECTOR_COMMAND_TIMED_OUT", routeExpected: "degraded"},
		{name: "running-outcome-lost", status: "running", commandExpected: "result_unknown", collectorExpected: "result_unknown", applyExpected: "unknown", code: "CONNECTOR_COMMAND_TIMED_OUT", routeExpected: "degraded"},
		{name: "already-verified-current", status: "running", commandExpected: "result_unknown", collectorExpected: "converged", applyExpected: "effective", initialCollector: "converged", initialApply: "effective", effective: 2, routeExpected: "active"},
		{name: "superseded-intent", status: "running", commandExpected: "result_unknown", collectorExpected: "installing", applyExpected: "pending", desired: 3, routeExpected: "active"},
	}
	for i := range scenarios {
		c := &scenarios[i]
		c.host = uuid.New()
		c.connector = uuid.New()
		c.collector = uuid.New()
		c.collection = uuid.New()
		distribution, target := uuid.New(), uuid.New()
		if c.desired == 0 {
			c.desired = 2
		}
		if c.effective == 0 {
			c.effective = 1
		}
		if c.initialCollector == "" {
			c.initialCollector = "installing"
		}
		if c.initialApply == "" {
			c.initialApply = "pending"
		}
		exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash)VALUES($1,$2,$3,'127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", c.host, enterprise, c.name)
		exec("INSERT INTO connectors(id,enterprise_id,role,name,host_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at)VALUES($1,$2,'host',$3,$4,$5,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'online',1,now()+interval '1 hour')", c.connector, enterprise, c.name, c.host, c.connector.String())
		exec("UPDATE hosts SET connector_id=$2 WHERE id=$1", c.host, c.connector)
		exec("INSERT INTO connector_sessions(connector_id,enterprise_id,gateway_instance_id,connection_epoch,capabilities,connected_at,last_heartbeat_at)VALUES($1,$2,'owned-timeout',1,'{}',now(),now())", c.connector, enterprise)
		exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision)VALUES($1,$2,'fixture','fixture','fixture','supported','[]',1)", distribution, distribution.String())
		exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision,effective_revision)VALUES($1,$2,'host',$3,$4,'linux_amd64','leaf',$5,$6,$7)", c.collector, enterprise, c.host, distribution, c.initialCollector, c.desired, c.effective)
		exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by)VALUES($1,$2,$3,'probe','http','{}',$4)", target, enterprise, c.name, creator)
		exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,effective_collector_id,desired_revision,effective_revision,assignment_epoch,effective_assignment_epoch,created_by,apply_status)VALUES($1,$2,$3,'http','monitoring_target',$4,$5,$5,$7,$8,1,1,$6,$9)", c.collection, enterprise, c.name, target, c.collector, creator, c.desired, c.effective, c.initialApply)
		exec("INSERT INTO monitoring_collection_assignments(collection_id,enterprise_id,epoch,collector_id,activated_at)VALUES($1,$2,1,$3,now())", c.collection, enterprise, c.collector)
		for revision := int64(1); revision <= c.desired; revision++ {
			status := "effective"
			if revision > c.effective {
				status = "applying"
			}
			bundle, _ := json.Marshal(configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, CollectorID: c.collector.String(), Collections: []configbundle.Collection{{ID: c.collection.String(), Integration: "http", TargetID: target.String(), TargetType: "monitoring_target", Revision: revision, AssignmentEpoch: 1, Enabled: true}}})
			exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status)VALUES($1,$2,$3,'{}',$4,decode(repeat('00',32),'hex'),$5)", uuid.New(), c.collector, revision, bundle, status)
		}
		payload, _ := protojson.Marshal(&connectorv1.CollectorManagementCommand{CollectorId: c.collector.String(), ResourceId: c.host.String(), ResourceType: "host", Operation: "configure", DesiredRevision: 2, Transport: "direct", ConfigSha256: strings.Repeat("00", 32)})
		exec("INSERT INTO connector_commands(id,command_id,enterprise_id,connector_id,connection_epoch,operation_ref,command_type,payload_schema_version,payload,payload_hash,idempotency_key,expires_at,status)VALUES($1,$2,$3,$4,1,$2,'collector_management','fixture',$5,decode(repeat('00',32),'hex'),$2,now()-interval '1 second',$6)", uuid.New(), c.name, enterprise, c.connector, payload, c.status)
		exec("INSERT INTO telemetry_routes(id,enterprise_id,collector_id,kind,transport,status)VALUES($1,$2,$3,'direct_argus','direct','active')", uuid.New(), enterprise, c.collector)
	}
	if err := (Service{Store: store}).SweepCommandTimeouts(ctx); err != nil {
		t.Fatal(err)
	}
	results := []map[string]any{}
	for _, c := range scenarios {
		var commandStatus, collectorStatus, applyStatus, code, route string
		var effective int64
		err := store.Pool.QueryRow(ctx, "SELECT cmd.status,col.status,item.apply_status,item.error_code,col.effective_revision,r.status FROM connector_commands cmd JOIN collector_instances col ON col.id=$2 JOIN telemetry_routes r ON r.collector_id=col.id JOIN monitoring_collections item ON item.id=$3 WHERE cmd.enterprise_id=$1 AND cmd.command_id=$4", enterprise, c.collector, c.collection, c.name).Scan(&commandStatus, &collectorStatus, &applyStatus, &code, &effective, &route)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, map[string]any{"name": c.name, "command_status": commandStatus, "collector_status": collectorStatus, "apply_status": applyStatus, "error_code": code, "effective_revision": effective, "route_status": route})
		if commandStatus != c.commandExpected || collectorStatus != c.collectorExpected || applyStatus != c.applyExpected || code != c.code || effective != c.effective || route != c.routeExpected {
			t.Errorf("expired command left unresolved or fabricated state: %s command=%s collector=%s apply=%s code=%s effective=%d route=%s", c.name, commandStatus, collectorStatus, applyStatus, code, effective, route)
		}
	}
	late := scenarios[2]
	service := Service{Store: store}
	identity := TrustedIdentity{EnterpriseID: enterprise, ConnectorID: late.connector}
	outcome := &connectorv1.CollectorManagementResult{CollectorId: late.collector.String(), EffectiveRevision: 2, AppliedConfigSha256: strings.Repeat("11", 32), Status: "converged"}
	raw, _ := protojson.Marshal(outcome)
	if _, err := service.TransitionCommand(ctx, identity, 1, late.name, "succeeded", raw, ""); err == nil {
		t.Fatal("late result with another configuration hash was accepted")
	}
	outcome.AppliedConfigSha256 = strings.Repeat("00", 32)
	raw, _ = protojson.Marshal(outcome)
	if _, err := service.TransitionCommand(ctx, identity, 1, late.name, "succeeded", raw, ""); err != nil {
		t.Fatal("verified late outcome could not recover unknown state", err)
	}
	var status, apply, config, route string
	var errorCode, configError *string
	var effective int64
	var fabricatedSample bool
	if err := store.Pool.QueryRow(ctx, "SELECT c.status,c.effective_revision,i.apply_status,r.status,f.status,cmd.error_code,i.last_sample_at IS NOT NULL,f.failure_code FROM collector_instances c JOIN monitoring_collections i ON i.id=$2 JOIN telemetry_routes r ON r.collector_id=c.id JOIN collector_config_revisions f ON f.collector_id=c.id AND f.revision=2 JOIN connector_commands cmd ON cmd.command_id=$3 AND cmd.enterprise_id=c.enterprise_id WHERE c.id=$1", late.collector, late.collection, late.name).Scan(&status, &effective, &apply, &route, &config, &errorCode, &fabricatedSample, &configError); err != nil {
		t.Fatal(err)
	}
	if status != "converged" || effective != 2 || apply != "effective" || route != "active" || config != "effective" || errorCode != nil || configError != nil || fabricatedSample {
		t.Fatal("late recovery left inconsistent configuration or fabricated a sample", status, effective, apply, route, config, errorCode != nil, fabricatedSample)
	}
	results = append(results, map[string]any{"name": "verified_late_completion", "collector_status": status, "effective_revision": effective, "apply_status": apply, "config_status": config, "route_status": route, "sample_fabricated": fabricatedSample})
	if path := os.Getenv("ARGUS_PLAN8_TIMEOUT_EVIDENCE"); path != "" {
		raw, _ := json.MarshalIndent(results, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
