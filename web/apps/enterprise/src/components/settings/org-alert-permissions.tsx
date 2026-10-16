import type { Role } from "@argus/api-client";
import { Checkbox, DataTable, type Column } from "@argus/ui";
import { roleDisplayName } from "../../lib/role-presentation";
import { useTranslation } from "react-i18next";

const capabilities = [
  { id: "alert.read", label: "alerts.readPermission" },
  { id: "alert.policy.manage", label: "alerts.policyPermission" },
  { id: "alert.silence.manage", label: "alerts.silencePermission" },
  { id: "alert.notification.manage", label: "alerts.channelPermission" },
  { id: "alert.handle", label: "alerts.handlePermission" },
  { id: "audit.read", label: "alerts.auditPermission" },
  { id: "role.manage", label: "alerts.iamPermission" },
] as const;

// The existing IAM role definitions remain authoritative. This read-only
// comparison adds no grants; edits still use the existing organization form.
export function OrgAlertPermissions({ roles }: { roles: Role[] }) {
  const { t } = useTranslation();
  const columns: Column<(typeof capabilities)[number]>[] = [
    {
      key: "label",
      header: t("alerts.permission"),
      noWrap: true,
      render: (row) => (
        <>
          <div className="argus-alert-permission-name">{t(row.label)}</div>
          <code>{row.id}</code>
        </>
      ),
    },
    ...roles.map((role) => ({
      key: role.id,
      align: "center" as const,
      header: roleDisplayName(role, t),
      render: (row: (typeof capabilities)[number]) => (
        <Checkbox
          appearance="outline"
          isDisabled
          isSelected={
            role.permissions.includes("*") || role.permissions.includes(row.id)
          }
          aria-label={`${roleDisplayName(role, t)} ${t(row.label)}`}
        />
      ),
    })),
  ];
  return (
    <section
      className="argus-alert-permissions"
      aria-label={t("alerts.permissions")}
    >
      <h3>{t("alerts.permissions")}</h3>
      <DataTable
        density="dense"
        surface="plain"
        columns={columns}
        data={[...capabilities]}
        getRowKey={(row) => row.id}
      />
    </section>
  );
}
