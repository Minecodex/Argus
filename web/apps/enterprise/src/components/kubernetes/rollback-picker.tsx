import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  kubernetesRollbackHistoryType,
  kubernetesRollbackReference,
  type KubernetesInventoryObject,
  type KubernetesRollbackReference,
} from "@argus/api-client";
import { ResourcePickerDialog } from "@argus/ui";
import { useInventoryError } from "./inventory-error";

export function RollbackPicker({
  clusterId,
  namespaceId,
  workload,
  onSelect,
  onClose,
}: {
  clusterId: string;
  namespaceId: string;
  workload: KubernetesInventoryObject;
  onSelect: (reference: KubernetesRollbackReference) => void;
  onClose: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    formatError = useInventoryError();
  const [search, setSearch] = useState("");
  const [cursors, setCursors] = useState<Array<string | undefined>>([
    undefined,
  ]);
  const query = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "rollback-history",
      workload.uid,
      namespaceId,
      search,
      cursors.at(-1),
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.list(
        clusterId,
        {
          type: kubernetesRollbackHistoryType(workload.definition.key)!,
          namespace_mode: "explicit",
          namespace_ids: [namespaceId],
          name: search,
          limit: 30,
          cursor: cursors.at(-1),
        },
        signal,
      ),
  });
  const rows = (query.error ? [] : (query.data?.items ?? [])).flatMap(
    (object) => {
      const reference = kubernetesRollbackReference(workload, object);
      return reference ? [{ ...reference, created_at: object.created_at }] : [];
    },
  );
  return (
    <ResourcePickerDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("kubernetesWorkbench.mutation.chooseRevision")}
      description={t("kubernetesWorkbench.mutation.revisionPickerHint")}
      data={rows}
      columns={[
        { key: "revision", header: t("kubernetesWorkbench.mutation.revision") },
        { key: "name", header: t("kubernetesWorkbench.name") },
        { key: "resource_version", header: t("kubernetesWorkbench.version") },
      ]}
      getRowKey={(row) => row.uid}
      getRowLabel={(row) => `${row.revision} · ${row.name}`}
      search={search}
      onSearch={(value) => {
        setSearch(value);
        setCursors([undefined]);
      }}
      loading={query.isFetching}
      error={formatError(query.error)}
      page={cursors.length}
      hasPrevious={cursors.length > 1}
      hasNext={!!query.data?.next_cursor}
      onPrevious={() => setCursors((value) => value.slice(0, -1))}
      onNext={() => setCursors((value) => [...value, query.data?.next_cursor])}
      isDisabled={() => query.isFetching}
      onSelect={(row) =>
        onSelect({
          name: row.name,
          uid: row.uid,
          resource_version: row.resource_version,
          revision: row.revision,
        })
      }
    />
  );
}
