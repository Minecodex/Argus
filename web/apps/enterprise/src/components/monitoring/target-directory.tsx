import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type MonitoringTarget,
} from "@argus/api-client";
import {
  ActionMenu,
  Button,
  DataTable,
  EmptyState,
  FilterBar,
  Pagination,
  type Column,
} from "@argus/ui";
import type { useMonitoringActions } from "./actions";

export function TargetDirectory({
  canManage,
  actions,
  onOpenCollections,
}: {
  canManage: boolean;
  actions: ReturnType<typeof useMonitoringActions>;
  onOpenCollections: (id: string) => void;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const [cursors, setCursors] = useState<Array<string | undefined>>([
    undefined,
  ]);
  const cursor = cursors.at(-1);
  const query = useQuery({
    queryKey: ["monitoring", "targets", search, cursor],
    queryFn: () => api.monitoring.targets({ search, cursor, limit: 30 }),
  });
  const columns: Column<MonitoringTarget>[] = [
    {
      key: "name",
      header: t("monitoring.targetName"),
      render: (row) => (
        <Button
          variant="ghost"
          size="sm"
          onPress={() => onOpenCollections(row.id)}
        >
          {row.name}
        </Button>
      ),
    },
    { key: "engine", header: t("monitoring.integration") },
    {
      key: "identity",
      header: t("monitoring.endpoint"),
      render: (row) => row.identity.endpoint,
    },
  ];
  if (canManage) {
    columns.push({
      key: "actions",
      header: t("monitoring.actions"),
      render: (row) => (
        <ActionMenu
          name={row.name}
          isDisabled={actions.busy}
          items={[
            {
              label: t("monitoring.archiveTarget"),
              onSelect: () =>
                void actions.preview({
                  kind: "target",
                  operation: "archive",
                  id: row.id,
                  expected_version: row.version,
                }),
            },
          ]}
        />
      ),
    });
  }

  return (
    <>
      <FilterBar
        search={{
          value: search,
          placeholder: t("monitoring.searchTargets"),
          onChange: (value) => {
            setSearch(value);
            setCursors([undefined]);
          },
        }}
        onRefresh={() => void query.refetch()}
        refreshing={query.isFetching}
      />
      {query.isPending && <p role="status">{t("common.loading")}</p>}
      {query.error && (
        <p role="alert">
          {formatApiError(
            query.error,
            t("monitoring.errors.unavailable"),
            (id) => t("common.requestReference", { requestId: id }),
          )}
        </p>
      )}
      {query.data?.items.length ? (
        <DataTable
          data={query.data.items}
          columns={columns}
          getRowKey={(row) => row.id}
        />
      ) : (
        query.isSuccess && (
          <EmptyState
            title={t("monitoring.noActiveTargets")}
            description={t("monitoring.noActiveTargetsHint")}
          />
        )
      )}
      <Pagination
        page={cursors.length}
        hasNext={Boolean(query.data?.next_cursor)}
        isDisabled={query.isFetching}
        onChange={(page) =>
          setCursors((current) =>
            page <= current.length
              ? current.slice(0, page)
              : [...current, query.data?.next_cursor],
          )
        }
      />
    </>
  );
}
