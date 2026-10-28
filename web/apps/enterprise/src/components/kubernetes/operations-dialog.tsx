import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type KubernetesOperation,
} from "@argus/api-client";
import {
  Dialog,
  DataTable,
  Button,
  Pagination,
  Badge,
  CodeBlock,
  Spinner,
  EmptyState,
} from "@argus/ui";
import { KubernetesDrainProgressView } from "./drain-progress";
import { usePermission } from "../../lib/permissions";
export function KubernetesOperationsDialog({
  clusterId,
  onClose,
}: {
  clusterId: string;
  onClose: () => void;
}) {
  const api = useApi(),
    { t, i18n } = useTranslation(),
    [cursors, setCursors] = useState<Array<string | undefined>>([undefined]),
    [selected, setSelected] = useState<KubernetesOperation>();
  const canDrain = usePermission("kubernetes.node.drain"),
    canMaintain = usePermission("kubernetes.node.manage");
  const list = useQuery({
    queryKey: ["kubernetes-workbench", clusterId, "operations", cursors.at(-1)],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.operations(
        clusterId,
        { limit: 20, cursor: cursors.at(-1) },
        signal,
      ),
    refetchInterval: 3000,
  });
  const detail = useQuery({
    queryKey: ["kubernetes-workbench", clusterId, "operation", selected?.id],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.operation(clusterId, selected!.id, signal),
    enabled: !!selected,
    refetchInterval: 3000,
  });
  const cancel = useMutation({
    mutationFn: () =>
      api.kubernetesWorkbench.cancelDrain(
        clusterId,
        detail.data!.id,
        detail.data!.version,
      ),
    onSuccess: () => {
      void list.refetch();
      void detail.refetch();
    },
  });
  const rows = list.error ? [] : (list.data?.items ?? []);
  return (
    <Dialog
      open
      size="lg"
      title={t("kubernetesWorkbench.mutation.history")}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {list.isPending && <Spinner label={t("common.loading")} />}
      {list.error && (
        <p role="alert">
          {formatApiError(
            list.error,
            t("kubernetesWorkbench.errors.unavailable"),
            (id) => t("common.requestReference", { requestId: id }),
          )}
        </p>
      )}
      {rows.length ? (
        <DataTable
          data={rows}
          getRowKey={(r) => r.id}
          columns={[
            { key: "name", header: t("kubernetesWorkbench.name") },
            {
              key: "operation",
              header: t("kubernetesWorkbench.mutation.operation"),
              render: (r) =>
                r.operation.startsWith("helm_")
                  ? t(`kubernetesHelm.actions.${r.operation.slice(5)}`)
                  : t(`kubernetesWorkbench.mutation.actions.${r.operation}`),
            },
            {
              key: "status",
              header: t("kubernetesWorkbench.mutation.status"),
              render: (r) => (
                <Badge>
                  {t(`kubernetesWorkbench.mutation.statuses.${r.status}`)}
                </Badge>
              ),
            },
            {
              key: "updated_at",
              header: t("kubernetesWorkbench.observed"),
              render: (r) =>
                new Date(r.updated_at).toLocaleString(i18n.language),
            },
            {
              key: "actions",
              header: t("monitoring.actions"),
              render: (r) => (
                <Button size="sm" onPress={() => setSelected(r)}>
                  {t("kubernetesWorkbench.details")}
                </Button>
              ),
            },
          ]}
        />
      ) : (
        !list.isPending && (
          <EmptyState
            title={t("kubernetesWorkbench.mutation.empty")}
            description={t("kubernetesWorkbench.mutation.previewHint")}
          />
        )
      )}
      <Pagination
        page={cursors.length}
        hasNext={!!list.data?.next_cursor}
        isDisabled={list.isFetching}
        onChange={(page) =>
          setCursors((old) =>
            page <= old.length
              ? old.slice(0, page)
              : [...old, list.data?.next_cursor],
          )
        }
      />
      {selected && (
        <section aria-label={t("kubernetesWorkbench.details")}>
          {detail.error ? (
            <p role="alert">
              {formatApiError(
                detail.error,
                t("kubernetesWorkbench.errors.unavailable"),
                (id) => t("common.requestReference", { requestId: id }),
              )}
            </p>
          ) : (
            detail.data && (
              <>
                <p>
                  {t(
                    `kubernetesWorkbench.mutation.statuses.${detail.data.status}`,
                  )}{" "}
                  · {detail.data.error_code}
                </p>
                {detail.data.status === "result_unknown" && (
                  <p>{t("kubernetesWorkbench.errors.mutationUnknown")}</p>
                )}
                {detail.data.operation === "drain" && (
                  <>
                    {detail.data.status === "cancelled" ? (
                      <p>{t("kubernetesWorkbench.drain.cancelledHint")}</p>
                    ) : (
                      canDrain &&
                      canMaintain &&
                      [
                        "queued",
                        "running",
                        "observing",
                        "result_unknown",
                      ].includes(detail.data.status) && (
                        <Button
                          isDisabled={cancel.isPending}
                          onPress={() => cancel.mutate()}
                        >
                          {t("kubernetesWorkbench.drain.cancel")}
                        </Button>
                      )
                    )}
                    {cancel.error && (
                      <p role="alert">
                        {formatApiError(
                          cancel.error,
                          t("kubernetesWorkbench.errors.unavailable"),
                          (id) =>
                            t("common.requestReference", { requestId: id }),
                        )}
                      </p>
                    )}
                  </>
                )}
                {detail.data.result.drain ? (
                  <KubernetesDrainProgressView
                    progress={detail.data.result.drain}
                  />
                ) : (
                  <CodeBlock
                    code={JSON.stringify(detail.data.result, null, 2)}
                    language="json"
                  />
                )}
              </>
            )
          )}
        </section>
      )}
    </Dialog>
  );
}
