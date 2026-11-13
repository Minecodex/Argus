import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  ApiError,
  type DashboardItem,
  type DashboardSchemas,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Dialog,
  Button,
  DataTable,
  SearchInput,
  Spinner,
  Alert,
  Pagination,
} from "@argus/ui";
import { PendingActionConfirm } from "../hosts/pending-action-confirm";
export function OverviewDashboardSelector({
  clusterId,
  selection,
  onClose,
}: {
  clusterId: string;
  selection: DashboardSchemas["KubernetesOverviewSelection"];
  onClose: () => void;
}) {
  const api = useApi(),
    cache = useQueryClient(),
    { t } = useTranslation(),
    [search, setSearch] = useState(""),
    [cursors, setCursors] = useState<(string | undefined)[]>([undefined]),
    [pending, setPending] = useState<PendingActionPublic>(),
    [error, setError] = useState("");
  const query = useQuery({
    queryKey: [
      "kubernetes-overview-candidates",
      clusterId,
      search,
      cursors.at(-1),
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.overviewCandidates(
        clusterId,
        { search, cursor: cursors.at(-1), limit: 30 },
        signal,
      ),
  });
  const choose = async (board: DashboardItem, reset = false) => {
    setError("");
    try {
      setPending(
        await api.kubernetesWorkbench.previewOverview(clusterId, {
          dashboard_id: board.id,
          expected_dashboard_version: board.version,
          expected_selection_version: selection.selection_version,
          expected_resource_version: selection.resource_version,
          reset,
        }),
      );
    } catch (cause) {
      if (
        cause instanceof ApiError &&
        cause.code === "DASHBOARD_VERSION_CONFLICT"
      ) {
        void cache.invalidateQueries({
          queryKey: ["kubernetes-overview", clusterId],
        });
        void query.refetch();
      }
      setError(
        formatApiError(cause, t("dashboards.failed"), (id) =>
          t("common.requestReference", { requestId: id }),
        ),
      );
    }
  };
  return (
    <Dialog
      open
      size="lg"
      title={t("kubernetesResources.chooseDashboard")}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {pending ? (
        <PendingActionConfirm
          action={pending}
          onError={(cause) => {
            if (
              cause instanceof ApiError &&
              ["DASHBOARD_VERSION_CONFLICT", "ACTION_INVALIDATED"].includes(
                cause.code,
              )
            ) {
              setPending(undefined);
              setError(t("kubernetesResources.replaceWarning"));
              void cache.invalidateQueries({
                queryKey: ["kubernetes-overview", clusterId],
              });
              void query.refetch();
              return true;
            }
            return false;
          }}
          onDone={() => {
            void cache.invalidateQueries({
              queryKey: ["kubernetes-overview", clusterId],
            });
            onClose();
          }}
          onCancel={onClose}
          onDismiss={onClose}
        />
      ) : (
        <div className="argus-k8s-resource-detail">
          <SearchInput
            aria-label={t("dashboards.searchName")}
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setCursors([undefined]);
            }}
          />
          {error && (
            <Alert
              tone="danger"
              title={t("dashboards.failed")}
              description={error}
            />
          )}
          {query.isFetching && <Spinner label={t("common.loading")} />}
          {query.error && (
            <Alert
              tone="danger"
              title={t("dashboards.failed")}
              description={formatApiError(
                query.error,
                t("dashboards.failed"),
                (id) => t("common.requestReference", { requestId: id }),
              )}
            />
          )}
          {query.data && (
            <>
              <DataTable
                data={query.data.items}
                getRowKey={(row) => row.id}
                columns={[
                  { key: "name", header: t("dashboards.name") },
                  { key: "description", header: t("dashboards.description") },
                  {
                    key: "actions",
                    header: t("monitoring.actions"),
                    render: (row) => (
                      <Button
                        isDisabled={row.id === selection.dashboard?.id}
                        onPress={() => void choose(row)}
                      >
                        {t("common.select", { defaultValue: "Select" })}
                      </Button>
                    ),
                  },
                ]}
              />
              <Pagination
                page={cursors.length}
                totalPages={cursors.length + (query.data.next_cursor ? 1 : 0)}
                onChange={(page) =>
                  setCursors((old) =>
                    page <= old.length
                      ? old.slice(0, page)
                      : [...old, query.data?.next_cursor],
                  )
                }
              />
            </>
          )}
          <Button
            isDisabled={selection.mode === "default"}
            onPress={() => void choose(selection.default_dashboard, true)}
          >
            {t("kubernetesResources.resetDashboard")}
          </Button>
        </div>
      )}
    </Dialog>
  );
}
