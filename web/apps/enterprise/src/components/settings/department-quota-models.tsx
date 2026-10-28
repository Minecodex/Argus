import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ActionMenu, DataTable, QueryBoundary } from "@argus/ui";
import { QuotaDrawer } from "./model-quota-drawer";
import { useModelGovernanceMetadata } from "./use-model-governance-metadata";

export function DepartmentQuotaModels({
  onDashboard,
}: {
  onDashboard(id: string): void;
}) {
  const { t } = useTranslation();
  const query = useModelGovernanceMetadata();
  const [model, setModel] = useState<{ id: string; name: string }>();
  return (
    <QueryBoundary query={query}>
      <DataTable
        data={query.data?.models ?? []}
        getRowKey={(item) => item.id}
        columns={[
          { key: "name", header: t("aiSettings.model.name") },
          {
            key: "actions",
            header: t("settings.common.actions"),
            render: (item) => (
              <ActionMenu
                items={[
                  {
                    label: t("aiSettings.model.quota"),
                    onSelect: () => setModel(item),
                  },
                  {
                    label: t("modelGovernance.open"),
                    onSelect: () => onDashboard(item.id),
                  },
                ]}
              />
            ),
          },
        ]}
      />
      {model && (
        <QuotaDrawer model={model} onClose={() => setModel(undefined)} />
      )}
    </QueryBoundary>
  );
}
