-- name: GetWorkspaceResultArtifact :one
SELECT * FROM workspace_result_artifacts WHERE workspace_id=$1 AND enterprise_id=$2 AND artifact_id=$3;

-- name: GetWorkspaceResultArtifactByPath :one
SELECT * FROM workspace_result_artifacts WHERE workspace_id=$1 AND enterprise_id=$2 AND path=$3;

-- name: ReserveWorkspaceResultArtifact :one
INSERT INTO workspace_result_artifacts(workspace_id,enterprise_id,artifact_id,path,metadata)
VALUES($1,$2,$3,$4,$5) RETURNING *;

-- name: CompleteWorkspaceResultArtifact :execrows
UPDATE workspace_result_artifacts SET workspace_file_id=$4
WHERE workspace_id=$1 AND enterprise_id=$2 AND artifact_id=$3 AND (workspace_file_id IS NULL OR workspace_file_id=$4);

-- name: ListWorkspaceResultArtifacts :many
SELECT a.* FROM workspace_result_artifacts a
JOIN workspace_files f ON f.id=a.workspace_file_id AND f.workspace_id=a.workspace_id AND f.enterprise_id=a.enterprise_id AND f.deleted_at IS NULL
WHERE a.workspace_id=$1 AND a.enterprise_id=$2 ORDER BY a.created_at,a.artifact_id;

-- name: UpsertWorkspaceResultsMetadataFile :one
INSERT INTO workspace_files(id,enterprise_id,workspace_id,conversation_id,name,path,byte_size,content_hash,media_type,source_result_refs)
VALUES($1,$2,$3,$4,'meta.json','results/meta.json',$5,$6,'application/json',$7)
ON CONFLICT(workspace_id,path) WHERE deleted_at IS NULL DO UPDATE
SET byte_size=excluded.byte_size,content_hash=excluded.content_hash,media_type=excluded.media_type,source_result_refs=excluded.source_result_refs
RETURNING *;
