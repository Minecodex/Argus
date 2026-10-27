-- +goose Up
INSERT INTO permissions(id,description,registry_version) VALUES
 ('kubernetes.workload.privileged','Run host-privileged workloads within an authorized cluster',16),
 ('kubernetes.rbac.escalate','Explicitly delegate Kubernetes API permissions within an authorized cluster',16),
 ('kubernetes.service_account.use','Run workloads with service account API identities in an authorized namespace',16)
ON CONFLICT(id) DO UPDATE SET description=EXCLUDED.description,registry_version=EXCLUDED.registry_version;

-- +goose Down
DELETE FROM role_permissions WHERE permission_id IN ('kubernetes.workload.privileged','kubernetes.rbac.escalate','kubernetes.service_account.use');
DELETE FROM permissions WHERE id IN ('kubernetes.workload.privileged','kubernetes.rbac.escalate','kubernetes.service_account.use');
