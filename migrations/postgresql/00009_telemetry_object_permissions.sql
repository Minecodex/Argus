-- +goose Up
-- Retire signal/field privileges. Existing object-read permissions and data
-- grants are unchanged; old privileges never implicitly grant broader access.
WITH affected_roles AS (
 DELETE FROM role_permissions WHERE permission_id IN ('telemetry.query.metrics','telemetry.query.logs','telemetry.query.traces','telemetry.sensitive_fields.read')
 RETURNING role_id
), changed_roles AS (
 UPDATE roles SET version=version+1,updated_at=now() WHERE id IN (SELECT role_id FROM affected_roles) RETURNING id
), changed_users AS (
 UPDATE enterprise_users u SET authorization_version=authorization_version+1,updated_at=now()
 WHERE EXISTS(SELECT 1 FROM role_bindings b JOIN affected_roles r ON r.role_id=b.role_id
  WHERE b.enterprise_id=u.enterprise_id AND ((b.subject_type='user' AND b.subject_id=u.id) OR (b.subject_type='department' AND b.subject_id=u.department_id)))
 RETURNING id
)
UPDATE service_accounts s SET authorization_version=authorization_version+1,updated_at=now()
WHERE EXISTS(SELECT 1 FROM role_bindings b JOIN affected_roles r ON r.role_id=b.role_id
 WHERE b.enterprise_id=s.enterprise_id AND b.subject_type='service_account' AND b.subject_id=s.id);
DELETE FROM permissions WHERE id IN ('telemetry.query.metrics','telemetry.query.logs','telemetry.query.traces','telemetry.sensitive_fields.read');
UPDATE permissions SET registry_version=11;

-- +goose Down
-- Do not restore retired privileges or grants on rollback.
SELECT 1;
