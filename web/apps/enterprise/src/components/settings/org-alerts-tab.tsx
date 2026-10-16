import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useApi,
  formatApiError,
  type PendingActionPublic,
} from "@argus/api-client";
import { Button, Field, Select, Dialog, Skeleton } from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { PendingActionConfirm } from "../hosts/pending-action-confirm";
import {
  AlertCallout,
  AlertTable,
  AlertStatus,
  clock,
  useAlertText,
} from "../alerts/common";
export function OrgAlertsTab({ audit = false }: { audit?: boolean }) {
  const api = useApi(),
    cache = useQueryClient(),
    { t } = useAlertText();
  const read = usePermission(audit ? "audit.read" : "service_account.read"),
    manage = usePermission("service_account.manage");
  const [id, setId] = useState(""),
    [pending, setPending] = useState<PendingActionPublic | null>(null),
    [error, setError] = useState("");
  const settings = useQuery({
    queryKey: ["alerts", "configurations", "settings"],
    queryFn: () => api.alerts.configurations("settings"),
    enabled: read && !audit,
  });
  const accounts = useQuery({
    queryKey: ["org", "service-accounts"],
    queryFn: () => api.org.listServiceAccounts(),
    enabled: read && !audit,
  });
  const records = useQuery({
    queryKey: ["audit", "alert"],
    queryFn: () => api.audit.list({ resourceType: "alert" }),
    enabled: read && audit,
  });
  const row = settings.data?.[0];
  if (!read) return <AlertCallout danger>{t("alerts.forbidden")}</AlertCallout>;
  if (audit)
    return (
      <>
        <h3 className="argus-sr-only">{t("alerts.auditTab")}</h3>
        {records.isError && (
          <AlertCallout danger>
            {formatApiError(records.error, t("alerts.invalid"), (id) => id)}
          </AlertCallout>
        )}
        {records.isPending ? (
          <Skeleton height={120} />
        ) : (
          <AlertTable
            data={records.data?.items ?? []}
            columns={[
              {
                key: "createdAt",
                header: t("alerts.auditTime"),
                cellClassName: "argus-table__cell--small argus-table__cell--mono",
                render: (r) => (
                  <span className="argus-alert-mono">
                    {clock(r.createdAt, true)}
                  </span>
                ),
              },
              {
                key: "actorUserId",
                header: t("alerts.actor"),
                render: (r) => r.actorName ?? r.actorUserId ?? "—",
              },
              {
                key: "action",
                header: t("alerts.operation"),
                render: (r) => r.summary || r.action,
              },
              {
                key: "resourceId",
                header: t("alerts.object"),
                render: (r) => (
                  <code>{r.resourceName ?? r.resourceId ?? "—"}</code>
                ),
              },
              {
                key: "origin",
                header: t("alerts.origin"),
                render: (r) => (
                  <AlertStatus
                    value={r.origin === "admin_chatbox" ? "model" : "manual"}
                  />
                ),
              },
              {
                key: "result",
                header: t("alerts.outcome"),
                render: (r) => <AlertStatus value={r.result} />,
              },
            ]}
          />
        )}
      </>
    );
  return (
    <>
      <AlertCallout>{t("alerts.evaluatorHint")}</AlertCallout>
      {error && <AlertCallout danger>{error}</AlertCallout>}
      {settings.isError && (
        <AlertCallout danger>
          {formatApiError(settings.error, t("alerts.invalid"), (id) => id)}
        </AlertCallout>
      )}
      <Field label={t("alerts.evaluator")} requirement="required">
        <Select
          value={
            id ||
            (row?.spec as { evaluator_id: string } | undefined)?.evaluator_id ||
            ""
          }
          options={(accounts.data ?? [])
            .filter((a) => a.status === "active")
            .map((a) => ({ value: a.id, label: a.name }))}
          onValueChange={setId}
          disabled={!manage}
        />
      </Field>
      <Button
        variant="primary"
        isDisabled={!manage || !id}
        onPress={() =>
          void (async () => {
            try {
              setPending(
                await api.alerts.preview({
                  kind: "settings",
                  operation: row ? "update" : "create",
                  id: row?.id,
                  expected_version: row?.version ?? 0,
                  name: t("alerts.evaluator"),
                  spec: { evaluator_id: id },
                }),
              );
              setError("");
            } catch (e) {
              setError(formatApiError(e, t("alerts.invalid"), (id) => id));
            }
          })()
        }
      >
        {t("alerts.preview")}
      </Button>
      {pending && (
        <Dialog
          open
          title={t("alerts.preview")}
          onOpenChange={(v) => {
            if (!v) setPending(null);
          }}
        >
          <PendingActionConfirm
            action={pending}
            onDone={() => {
              setPending(null);
              void cache.invalidateQueries({ queryKey: ["alerts"] });
            }}
            onCancel={() => setPending(null)}
          />
        </Dialog>
      )}
    </>
  );
}
