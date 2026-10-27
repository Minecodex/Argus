-- name: ActivateVerifiedTelemetryTunnelRoute :many
UPDATE telemetry_routes AS r SET status='active',last_tested_at=now(),version=r.version+1,updated_at=now()
FROM collector_instances c,collector_config_revisions cfg,telemetry_tunnels t,credentials credential,secrets secret
WHERE r.collector_id=c.id AND r.enterprise_id=c.enterprise_id
 AND cfg.collector_id=c.id AND cfg.revision=c.effective_revision AND cfg.status='effective' AND octet_length(cfg.config_hash)=32
 AND c.status='converged' AND c.effective_revision=c.desired_revision AND c.effective_revision>0
 AND t.id=sqlc.arg(tunnel_id) AND t.enterprise_id=sqlc.arg(enterprise_id) AND t.collector_id=c.id AND t.enterprise_id=c.enterprise_id
 AND t.epoch=sqlc.arg(epoch) AND t.fence=sqlc.arg(fence) AND t.lease_owner=sqlc.arg(lease_owner)
 AND t.status='established' AND t.lease_expires_at>now() AND r.transport=t.transport
 AND credential.id=t.credential_id AND credential.enterprise_id=t.enterprise_id AND credential.status='active' AND credential.version=t.credential_version
 AND secret.id=credential.secret_id AND secret.enterprise_id=credential.enterprise_id AND secret.status='active'
 AND (t.initiator='direct_executor' OR EXISTS(
  SELECT 1 FROM connectors x JOIN connector_sessions s ON s.connector_id=x.id AND s.enterprise_id=x.enterprise_id
  WHERE x.id=t.connector_id AND x.enterprise_id=t.enterprise_id AND x.status='online'
   AND x.connection_epoch=t.owner_connection_epoch AND s.connection_epoch=x.connection_epoch AND NOT s.draining
 ))
 AND r.status IN ('pending','testing','degraded')
RETURNING r.collector_id;
