-- name: GetDashboardTemplateHead :one
SELECT template.* FROM dashboard_template_heads head JOIN dashboard_template_versions template
 ON template.template_key=head.template_key AND template.version=head.version WHERE head.template_key=$1;

-- name: GetDashboardTemplateVersion :one
SELECT * FROM dashboard_template_versions WHERE template_key=$1 AND version=$2;

-- name: InsertDashboardTemplateVersion :exec
INSERT INTO dashboard_template_versions(template_key,version,runtime_version,name,description,integration_keys,spec,spec_hash,references_json)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(template_key,version) DO NOTHING;

-- name: PromoteDashboardTemplateHead :exec
INSERT INTO dashboard_template_heads(template_key,version) VALUES($1,$2)
ON CONFLICT(template_key) DO UPDATE SET version=EXCLUDED.version,updated_at=now()
 WHERE EXCLUDED.version>dashboard_template_heads.version;

-- name: ListDashboardTemplateHeads :many
SELECT template.* FROM dashboard_template_heads head JOIN dashboard_template_versions template
 ON template.template_key=head.template_key AND template.version=head.version ORDER BY template.template_key;

-- name: LockManagedDashboardTemplate :exec
SELECT pg_advisory_xact_lock(hashtextextended('argus-managed-dashboard:'||sqlc.arg(enterprise_id)::uuid::text||':'||sqlc.arg(template_key)::text,0));

-- name: AllowManagedDashboardWrite :exec
SELECT set_config('argus.managed_dashboard_write',sqlc.arg(dashboard_id)::uuid::text,true);

-- name: GetManagedDashboardInstance :one
SELECT * FROM managed_dashboard_instances WHERE enterprise_id=$1 AND template_key=$2;

-- name: GetManagedDashboardByID :one
SELECT * FROM managed_dashboard_instances WHERE enterprise_id=$1 AND dashboard_id=$2;

-- name: ListManagedDashboardInstances :many
SELECT * FROM managed_dashboard_instances WHERE enterprise_id=$1 ORDER BY template_key;

-- name: ListManagedTemplateEnginesForTargets :many
SELECT DISTINCT engine FROM monitoring_targets WHERE enterprise_id=$1 AND id=ANY(sqlc.arg(target_ids)::uuid[]) AND status='active' ORDER BY engine;

-- name: ListManagedTemplateOwnKeys :many
SELECT DISTINCT integration_key FROM monitoring_collections WHERE enterprise_id=sqlc.arg(enterprise_id) AND status='active'
 AND ((target_type='host' AND target_id=ANY(sqlc.arg(host_ids)::uuid[]))
 OR (target_type='kubernetes_cluster' AND target_id=ANY(sqlc.arg(cluster_ids)::uuid[])))
 AND integration_key=ANY(sqlc.arg(builtin_keys)::text[]) ORDER BY integration_key;

-- name: PutManagedDashboardInstance :exec
INSERT INTO managed_dashboard_instances(enterprise_id,template_key,dashboard_id,applied_version) VALUES($1,$2,$3,$4)
ON CONFLICT(enterprise_id,template_key) DO UPDATE SET applied_version=EXCLUDED.applied_version,updated_at=now()
 WHERE managed_dashboard_instances.dashboard_id=EXCLUDED.dashboard_id AND managed_dashboard_instances.applied_version<=EXCLUDED.applied_version;

-- name: SetDraftTemplateOrigin :exec
UPDATE dashboard_drafts SET source_template_key=$5,source_template_version=$6
WHERE enterprise_id=$1 AND id=$2 AND editor_subject_type=$3 AND editor_subject_id=$4 AND dashboard_id IS NULL AND status='editing';
