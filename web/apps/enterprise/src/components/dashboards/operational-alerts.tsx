import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Button, DataTable, Dialog, Pagination, EmptyState } from "@argus/ui";
export type OperationalAlert = {
  id: string;
  name: string;
  severity: string;
  lifecycle: string;
};
export function OperationalAlerts({
  items,
  total,
  complete,
  summary,
  pageSize = 20,
}: {
  items: OperationalAlert[];
  total: number;
  complete: boolean;
  summary: boolean;
  pageSize?: number;
}) {
  const { t } = useTranslation(),
    navigate = useNavigate(),
    [open, setOpen] = useState(false),
    [page, setPage] = useState(1);
  const pages = Math.max(1, Math.ceil(items.length / pageSize)),
    current = Math.min(page, pages);
  const list = (
    <div className="argus-resource-results">
      {!complete && <p role="status">{t("kubernetesResources.partial")}</p>}
      {items.length > 0 ? (
        <DataTable
          data={items.slice((current - 1) * pageSize, current * pageSize)}
          getRowKey={(row) => row.id}
          columns={[
            {
              key: "name",
              header: t("alerts.name"),
              render: (row) => (
                <Button
                  variant="ghost"
                  onPress={() =>
                    void navigate({
                      to: "/alerts",
                      search: { instance: row.id },
                    })
                  }
                >
                  {row.name}
                </Button>
              ),
            },
            { key: "severity", header: t("alerts.severity") },
          ]}
        />
      ) : (
        <EmptyState
          title={t("kubernetesResources.noAlerts")}
          description={t("kubernetesResources.liveSnapshot")}
        />
      )}
      {pages > 1 && (
        <Pagination page={current} totalPages={pages} onChange={setPage} />
      )}
    </div>
  );
  if (!summary) return list;
  return (
    <>
      <Button
        variant="ghost"
        className="argus-resource-summary__item"
        onPress={() => setOpen(true)}
      >
        <span>{t("kubernetesResources.activeAlerts")}</span>
        <strong>
          {complete ? "" : "≥ "}
          {total}
        </strong>
      </Button>
      <Dialog
        open={open}
        title={t("kubernetesResources.scopedAlerts")}
        onOpenChange={setOpen}
      >
        {list}
      </Dialog>
    </>
  );
}
