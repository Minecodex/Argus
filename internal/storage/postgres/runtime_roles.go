package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"
)

const RuntimeRolePolicyVersion = "argus.runtime_database_roles/v1"

type RuntimeRoleReceipt struct {
	PolicyVersion string   `json:"policy_version"`
	PolicySHA256  string   `json:"policy_sha256"`
	SchemaVersion int64    `json:"schema_version"`
	Roles         []string `json:"roles"`
}

type runtimeGrant struct {
	Role, Table, Privileges string
	Since                   int64
}

// The four trusted business/control services already share the complete
// application schema in the original deployment contract. Telemetry processes
// have separate, explicit capabilities; they never receive that broad grant.
var controlRuntimeRoles = []string{"argus_server", "argus_worker", "argus_gateway", "argus_direct_executor"}

func telemetryRuntimeGrants() []runtimeGrant {
	const ingest, query, writer = "argus_telemetry_ingest", "argus_telemetry_query", "argus_telemetry_writer"
	grants := []runtimeGrant{}
	add := func(role, privileges string, since int64, tables ...string) {
		for _, table := range tables {
			grants = append(grants, runtimeGrant{role, table, privileges, since})
		}
	}
	for _, role := range []string{ingest, query} {
		add(role, "SELECT", 1, "pki_trust_bundles")
		add(role, "SELECT,INSERT,UPDATE", 1, "pki_node_trust_acks")
	}
	add(ingest, "SELECT", 1, "collector_instances", "collector_distribution_versions", "collector_config_revisions", "telemetry_routes")
	add(ingest, "SELECT,INSERT,UPDATE", 1, "telemetry_certificates", "pki_certificate_identities")
	add(ingest, "SELECT,UPDATE(consumed_at)", 1, "telemetry_enrollment_tokens")
	add(ingest, "SELECT", 4, "telemetry_sources")
	// The collection reference trigger checks only resource identity columns,
	// including when observation fields change. Host addresses/credentials and
	// Kubernetes connection configuration remain unreadable to this role.
	add(ingest, "SELECT(id,enterprise_id)", 21, "hosts", "kubernetes_clusters")
	add(ingest, "SELECT", 21, "monitoring_applications", "monitoring_targets", "monitoring_collections", "monitoring_collection_revisions", "monitoring_collection_assignments", "monitoring_ingest_endpoints", "monitoring_ingest_keys", "kubernetes_namespace_resources")
	add(ingest, "SELECT,INSERT,UPDATE", 21, "monitoring_application_services", "monitoring_runtime_instances")
	add(ingest, "UPDATE(last_used_at)", 21, "monitoring_ingest_keys")
	add(ingest, "UPDATE(last_sample_at,last_attempt_at,observed_at,health,error_code)", 25, "monitoring_collections")
	add(ingest, "UPDATE(last_received_at,last_rejected_at,last_error_code)", 26, "monitoring_ingest_endpoints")
	add(ingest, "SELECT", 39, "telemetry_ingest_config_states")
	add(ingest, "SELECT", 48, "kubernetes_namespace_object_identities")
	add(ingest, "SELECT,INSERT,UPDATE", 49, "collector_delivery_reports")
	add(ingest, "SELECT", 49, "collector_delivery_policies")
	add(writer, "SELECT", 1, "enterprises", "enterprise_telemetry_tables")
	add(writer, "SELECT,INSERT,UPDATE", 1, "telemetry_retention_policies", "telemetry_usage_daily", "telemetry_dlq_records")
	add(query, "SELECT", 1, "enterprises", "pki_certificate_identities")
	add(query, "SELECT,INSERT,UPDATE", 1, "enterprise_telemetry_tables", "audit_chain_heads")
	// InsertAuditEvent returns the appended record, which also requires SELECT.
	// UPDATE/DELETE remain forbidden for this append-only query audit role.
	add(query, "SELECT,INSERT", 1, "audit_events")
	add(query, "SELECT", 21, "monitoring_applications", "monitoring_application_services", "monitoring_runtime_instances", "monitoring_targets", "kubernetes_namespace_resources", "monitoring_application_bindings", "monitoring_collections", "monitoring_collection_revisions", "monitoring_collection_assignments")
	return grants
}

// ReconcileRuntimeRolePermissions also supports an already-migrated database
// whose runtime accounts were created afterwards. It creates no accounts,
// changes no credentials or memberships and grants no superuser/DDL authority.
func ReconcileRuntimeRolePermissions(ctx context.Context, databaseURL string) (RuntimeRoleReceipt, error) {
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return RuntimeRoleReceipt{}, err
	}
	defer database.Close()
	return reconcileRuntimeRolePermissions(ctx, database)
}

func reconcileRuntimeRolePermissions(ctx context.Context, database *sql.DB) (RuntimeRoleReceipt, error) {
	if err := goose.SetDialect("postgres"); err != nil {
		return RuntimeRoleReceipt{}, err
	}
	version, err := goose.GetDBVersionContext(ctx, database)
	if err != nil {
		return RuntimeRoleReceipt{}, err
	}
	grants := telemetryRuntimeGrants()
	encoded, _ := json.Marshal(struct {
		Control   []string
		Telemetry []runtimeGrant
	}{controlRuntimeRoles, grants})
	digest := sha256.Sum256(encoded)
	receipt := RuntimeRoleReceipt{PolicyVersion: RuntimeRolePolicyVersion, PolicySHA256: hex.EncodeToString(digest[:]), SchemaVersion: version, Roles: []string{}}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext('argus-runtime-database-roles'))"); err != nil {
		return receipt, err
	}
	var databaseName string
	if err = tx.QueryRowContext(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
		return receipt, err
	}
	roles := append(append([]string{}, controlRuntimeRoles...), "argus_telemetry_ingest", "argus_telemetry_query", "argus_telemetry_writer")
	for index, role := range roles {
		var elevated bool
		err = tx.QueryRowContext(ctx, "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls FROM pg_roles WHERE rolname=$1", role).Scan(&elevated)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return receipt, err
		}
		if elevated {
			return receipt, fmt.Errorf("runtime database role %s has unexpected administrative privileges", role)
		}
		quoted := pgx.Identifier{role}.Sanitize()
		for _, statement := range []string{"GRANT CONNECT ON DATABASE " + pgx.Identifier{databaseName}.Sanitize() + " TO " + quoted, "GRANT USAGE ON SCHEMA public TO " + quoted} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return receipt, fmt.Errorf("initialize runtime database role %s: %w", role, err)
			}
		}
		if index < len(controlRuntimeRoles) {
			for _, statement := range []string{"GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO " + quoted, "GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO " + quoted} {
				if _, err = tx.ExecContext(ctx, statement); err != nil {
					return receipt, err
				}
			}
		} else {
			for _, grant := range grants {
				if grant.Role != role || version < grant.Since {
					continue
				}
				var table sql.NullString
				if err = tx.QueryRowContext(ctx, "SELECT to_regclass($1)::text", "public."+grant.Table).Scan(&table); err != nil {
					return receipt, err
				}
				if !table.Valid {
					return receipt, fmt.Errorf("runtime database role %s requires missing table %s at schema %d", role, grant.Table, version)
				}
				statement := "GRANT " + grant.Privileges + " ON " + pgx.Identifier{"public", grant.Table}.Sanitize() + " TO " + quoted
				if _, err = tx.ExecContext(ctx, statement); err != nil {
					return receipt, fmt.Errorf("reconcile runtime database role %s on %s: %w", role, grant.Table, err)
				}
			}
		}
		receipt.Roles = append(receipt.Roles, role)
	}
	if err = tx.Commit(); err != nil {
		return receipt, err
	}
	return receipt, nil
}
