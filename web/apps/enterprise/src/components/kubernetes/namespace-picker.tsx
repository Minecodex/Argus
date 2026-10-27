import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi } from "@argus/api-client";
import { ResourcePickerDialog } from "@argus/ui";
import { useInventoryError } from "./inventory-error";

export type NamespaceChoice = { id: string; name: string };

export function NamespacePicker({
  clusterId,
  onSelect,
  onClose,
}: {
  clusterId: string;
  onSelect: (value: NamespaceChoice) => void;
  onClose: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    formatError = useInventoryError();
  const [search, setSearch] = useState(""),
    [cursors, setCursors] = useState<Array<string | undefined>>([undefined]);
  const query = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "namespace-picker",
      search,
      cursors.at(-1),
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.list(
        clusterId,
        {
          type: "namespace",
          name: search,
          namespace_mode: "all_authorized",
          limit: 30,
          cursor: cursors.at(-1),
        },
        signal,
      ),
  });
  return (
    <ResourcePickerDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("kubernetesWorkbench.chooseNamespace")}
      description={t("kubernetesWorkbench.search")}
      data={query.error ? [] : (query.data?.items ?? [])}
      columns={[
        { key: "name", header: t("kubernetesWorkbench.namespace") },
        { key: "uid", header: t("kubernetesWorkbench.uid") },
      ]}
      getRowKey={(row) => row.uid}
      getRowLabel={(row) => row.name}
      search={search}
      onSearch={(value) => {
        setSearch(value);
        setCursors([undefined]);
      }}
      page={cursors.length}
      loading={query.isFetching}
      error={formatError(query.error)}
      hasPrevious={cursors.length > 1}
      hasNext={!!query.data?.next_cursor}
      onPrevious={() => setCursors((v) => v.slice(0, -1))}
      onNext={() => setCursors((v) => [...v, query.data?.next_cursor])}
      isDisabled={(row) => !query.data?.namespace_references[row.uid]}
      onSelect={(row) => {
        const id = query.data?.namespace_references[row.uid];
        if (id) onSelect({ id, name: row.name });
      }}
    />
  );
}
