-- name: ListDashboardFolders :many
SELECT * FROM dashboard_folders WHERE enterprise_id=$1 ORDER BY sort_order,name,id;

-- name: GetDashboardFolder :one
SELECT * FROM dashboard_folders WHERE id=$1 AND enterprise_id=$2;

-- name: LockDashboardFolder :one
SELECT * FROM dashboard_folders WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: CreateDashboardFolder :one
INSERT INTO dashboard_folders(id,enterprise_id,name,description,sort_order,created_by,updated_by)
VALUES($1,$2,$3,$4,$5,$6,$6) RETURNING *;

-- name: UpdateDashboardFolder :one
UPDATE dashboard_folders SET name=$3,description=$4,sort_order=$5,status=$6,updated_by=$7,version=version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND version=$8 RETURNING *;

-- name: CountFolderDashboards :one
SELECT count(*) FROM dashboards WHERE enterprise_id=$1 AND folder_id=$2;

-- name: ListDashboards :many
SELECT * FROM dashboards WHERE enterprise_id=$1 ORDER BY name,id;

-- name: GetDashboard :one
SELECT * FROM dashboards WHERE id=$1 AND enterprise_id=$2;

-- name: LockDashboard :one
SELECT * FROM dashboards WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: CreateDashboard :one
INSERT INTO dashboards(id,enterprise_id,folder_id,name,description,created_by,updated_by)
VALUES($1,$2,$3,$4,$5,$6,$6) RETURNING *;

-- name: ActivateDashboardRevision :one
UPDATE dashboards SET active_revision_id=$3,name=$4,description=$5,folder_id=$6,updated_by=$7,version=version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND version=$8 AND lifecycle='active' RETURNING *;

-- name: UpdateDashboardLifecycle :one
UPDATE dashboards SET lifecycle=$3,updated_by=$4,version=version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND version=$5 RETURNING *;

-- name: GetDashboardRevision :one
SELECT * FROM dashboard_revisions WHERE id=$1 AND enterprise_id=$2;

-- name: ListDashboardRevisions :many
SELECT * FROM dashboard_revisions WHERE dashboard_id=$1 AND enterprise_id=$2 ORDER BY revision_number DESC;

-- name: CreateDashboardRevision :one
INSERT INTO dashboard_revisions(id,enterprise_id,dashboard_id,revision_number,schema_version,name,description,folder_id,spec,spec_hash,validation_report,sample_report,created_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING *;

-- name: CreateDashboardDraft :one
INSERT INTO dashboard_drafts(id,enterprise_id,dashboard_id,editor_subject_type,editor_subject_id,base_revision_id,base_object_version,name,description,folder_id,spec,proposed_bindings)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING *;

-- name: GetDashboardDraft :one
SELECT * FROM dashboard_drafts WHERE id=$1 AND enterprise_id=$2 AND editor_subject_type=$3 AND editor_subject_id=$4;

-- name: LockDashboardDraft :one
SELECT * FROM dashboard_drafts WHERE id=$1 AND enterprise_id=$2 AND editor_subject_type=$3 AND editor_subject_id=$4 FOR UPDATE;

-- name: ListDashboardDrafts :many
SELECT * FROM dashboard_drafts WHERE enterprise_id=$1 AND editor_subject_type=$2 AND editor_subject_id=$3 AND status='editing' ORDER BY updated_at DESC,id;

-- name: SaveDashboardDraft :one
UPDATE dashboard_drafts SET name=$5,description=$6,folder_id=$7,spec=$8,proposed_bindings=$9,draft_version=draft_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND editor_subject_type=$3 AND editor_subject_id=$4 AND draft_version=$10 AND status='editing' RETURNING *;

-- name: RebaseDashboardDraft :one
UPDATE dashboard_drafts SET base_revision_id=$5,base_object_version=$6,draft_version=draft_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND editor_subject_type=$3 AND editor_subject_id=$4 AND draft_version=$7 AND status='editing' RETURNING *;

-- name: ConsumeDashboardDraft :one
UPDATE dashboard_drafts SET status='published',dashboard_id=$3,published_revision_id=$4,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND draft_version=$5 AND status='editing' RETURNING *;

-- name: DiscardDashboardDraft :execrows
UPDATE dashboard_drafts SET status='discarded',draft_version=draft_version+1,updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND editor_subject_type=$3 AND editor_subject_id=$4 AND draft_version=$5 AND status='editing';

-- name: ListDashboardBindings :many
SELECT * FROM dashboard_bindings WHERE enterprise_id=$1 AND dashboard_id=$2 ORDER BY created_at,id;

-- name: ListResourceDashboardBindings :many
SELECT * FROM dashboard_bindings WHERE enterprise_id=$1 AND resource_type=$2 AND resource_id=$3 ORDER BY created_at,id;

-- name: CreateDashboardBinding :one
INSERT INTO dashboard_bindings(id,enterprise_id,dashboard_id,resource_type,resource_id,created_by)
VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(enterprise_id,dashboard_id,resource_type,resource_id) DO UPDATE SET dashboard_id=EXCLUDED.dashboard_id RETURNING *;

-- name: DeleteDashboardBinding :execrows
DELETE FROM dashboard_bindings WHERE id=$1 AND enterprise_id=$2 AND version=$3;

-- name: FindDashboardBinding :one
SELECT * FROM dashboard_bindings WHERE enterprise_id=$1 AND dashboard_id=$2 AND resource_type=$3 AND resource_id=$4;

-- name: LockDashboardBindingHost :one
SELECT id,name,status,resource_version FROM hosts WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;

-- name: LockDashboardBindingCluster :one
SELECT id,name,status,resource_version FROM kubernetes_clusters WHERE id=$1 AND enterprise_id=$2 FOR UPDATE;
