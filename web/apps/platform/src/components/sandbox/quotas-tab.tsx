import { useQueries, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useApi } from "@argus/api-client";
import { DataTable, EmptyState, RowAction, Spinner } from "@argus/ui";
import { QuotaEditor } from "../quota-editor";

type QuotaRow = {
  enterpriseId: string;
  enterpriseName: string;
  maxConcurrentSessions: number;
  monthlySessionSeconds: number;
};

/** 企业配额 Tab：按企业聚合展示 Sandbox 配额，抽屉内编辑。 */
export function QuotasTab() {
  const { t } = useTranslation();
  const api = useApi();
  const [editing, setEditing] = useState<{
    id: string;
    name: string;
  } | null>(null);

  const enterprises = useQuery({
    queryKey: ["platform", "enterprises"],
    queryFn: () => api.platform.enterprises.list(),
  });

  const enterpriseItems = enterprises.data?.items ?? [];
  const quotas = useQueries({
    queries: enterpriseItems.map((enterprise) => ({
      queryKey: ["platform", "quota", enterprise.id],
      queryFn: () => api.platform.quotas.get(enterprise.id),
      retry: false,
    })),
  });

  const isPending =
    enterprises.isPending || quotas.some((query) => query.isPending);

  const rows: QuotaRow[] = enterpriseItems.flatMap((enterprise, index) => {
    const quota = quotas[index]?.data;
    if (!quota) return [];
    return [
      {
        enterpriseId: enterprise.id,
        enterpriseName: enterprise.name,
        maxConcurrentSessions: quota.maxConcurrentSessions,
        monthlySessionSeconds: quota.monthlySessionSeconds,
      },
    ];
  });

  return (
    <div className="argus-platform-stack">
      {isPending ? (
        <Spinner />
      ) : rows.length === 0 ? (
        <EmptyState description="" title={t("sandbox.quotas.empty")} />
      ) : (
        <DataTable<QuotaRow>
          columns={[
            {
              key: "enterpriseName",
              header: t("sandbox.quotas.table.enterprise"),
            },
            {
              key: "maxConcurrentSessions",
              header: t("sandbox.quotas.table.concurrent"),
              align: "right",
            },
            {
              key: "monthlySessionSeconds",
              header: t("sandbox.quotas.table.monthlySeconds"),
              align: "right",
            },
            {
              key: "enterpriseId",
              header: t("common.actions"),
              render: (row) => (
                <RowAction
                  onClick={() =>
                    setEditing({
                      id: row.enterpriseId,
                      name: row.enterpriseName,
                    })
                  }
                >
                  {t("sandbox.quotas.edit")}
                </RowAction>
              ),
            },
          ]}
          data={rows}
          getRowKey={(row) => row.enterpriseId}
        />
      )}

      {editing && (
        <QuotaEditor
          enterpriseId={editing.id}
          enterpriseName={editing.name}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}
