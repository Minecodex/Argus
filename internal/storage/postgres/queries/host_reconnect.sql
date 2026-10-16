-- name: ReauthorizeHostControlTunnel :one
UPDATE connector_control_tunnels SET credential_version=$3,status='desired',epoch=epoch+1,fence=fence+1,
lease_owner='',lease_expires_at=NULL,next_claim_at=NULL,reconnect_attempt=0,last_drop_reason='',updated_at=now()
WHERE id=$1 AND enterprise_id=$2 AND status<>'removed' RETURNING *;

-- name: ReauthorizeHostTelemetryTunnels :execrows
UPDATE telemetry_tunnels SET credential_version=$4,status='desired',epoch=epoch+1,fence=fence+1,
lease_owner='',owner_connection_epoch=0,lease_expires_at=NULL,next_claim_at=NULL,reconnect_attempt=0,last_drop_reason='',updated_at=now()
WHERE host_id=$1 AND enterprise_id=$2 AND credential_id=$3 AND initiator='direct_executor' AND status<>'removed';
