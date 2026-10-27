import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  type KubernetesHelmRelease,
  type KubernetesHelmPreviewInput,
} from "@argus/api-client";
import {
  Badge,
  Button,
  DataTable,
  Dialog,
  EmptyState,
  Input,
  Spinner,
} from "@argus/ui";
import { NamespacePicker, type NamespaceChoice } from "./namespace-picker";
import { usePermission } from "../../lib/permissions";
import { useInventoryError } from "./inventory-error";
import { HelmMutationDialog } from "./helm-mutation-dialog";

export function KubernetesHelmDialog({
  clusterId,
  onClose,
  onSubmitted,
}: {
  clusterId: string;
  onClose: () => void;
  onSubmitted: () => void;
}) {
  const api = useApi(),
    { t, i18n } = useTranslation(),
    formatError = useInventoryError();
  const canManage = usePermission("kubernetes.helm.manage");
  const [namespace, setNamespace] = useState<NamespaceChoice>(),
    [picker, setPicker] = useState(false),
    [search, setSearch] = useState("");
  const [selected, setSelected] = useState<KubernetesHelmRelease>();
  const [mutation, setMutation] = useState<{
    operation: KubernetesHelmPreviewInput["operation"];
    release?: KubernetesHelmRelease;
    revision?: number;
  }>();
  const list = useQuery({
    queryKey: ["kubernetes-workbench", clusterId, "helm", namespace?.id],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.helmReleases(clusterId, namespace!.id, signal),
    enabled: !!namespace,
    refetchInterval: 15000,
  });
  const history = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "helm-history",
      namespace?.id,
      selected?.name,
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.helmHistory(
        clusterId,
        namespace!.id,
        selected!.name,
        signal,
      ),
    enabled: !!namespace && !!selected,
    refetchInterval: 15000,
  });
  if (mutation && namespace)
    return (
      <HelmMutationDialog
        clusterId={clusterId}
        namespace={namespace}
        {...mutation}
        onClose={() => setMutation(undefined)}
        onSubmitted={() => {
          setMutation(undefined);
          onSubmitted();
        }}
      />
    );
  const rows = list.error
    ? []
    : (list.data?.items ?? []).filter(
        (r) => r.name.includes(search) || r.chart.includes(search),
      );
  return (
    <>
      <Dialog
        open
        size="lg"
        title={t("kubernetesHelm.title")}
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
      >
        <p>{t("kubernetesHelm.scopeHint")}</p>
        <div className="argus-k8s-workbench-toolbar">
          <Button onPress={() => setPicker(true)}>
            {namespace?.name ?? t("kubernetesWorkbench.chooseNamespace")}
          </Button>
          <Button
            isDisabled={!namespace || list.isFetching}
            onPress={() => {
              void list.refetch();
              void history.refetch();
            }}
          >
            {t("kubernetesWorkbench.refresh")}
          </Button>
          {canManage && (
            <Button
              variant="primary"
              isDisabled={!namespace}
              onPress={() => setMutation({ operation: "install" })}
            >
              {t("kubernetesHelm.actions.install")}
            </Button>
          )}
        </div>
        <Input
          aria-label={t("kubernetesHelm.search")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t("kubernetesHelm.search")}
        />
        {(list.isFetching || history.isFetching) && (
          <Spinner label={t("common.loading")} />
        )}
        {(list.error || history.error) && (
          <p role="alert">{formatError(list.error || history.error)}</p>
        )}
        {namespace &&
          (rows.length ? (
            <DataTable
              data={rows}
              getRowKey={(r) => r.name}
              columns={[
                { key: "name", header: t("kubernetesHelm.name") },
                {
                  key: "chart",
                  header: t("kubernetesHelm.chart"),
                  render: (r) => `${r.chart} ${r.chart_version}`,
                },
                { key: "revision", header: t("kubernetesHelm.revision") },
                {
                  key: "status",
                  header: t("kubernetesWorkbench.mutation.status"),
                  render: (r) => (
                    <Badge>
                      {t(`kubernetesHelm.statuses.${r.status}`, {
                        defaultValue: r.status,
                      })}
                    </Badge>
                  ),
                },
                {
                  key: "actions",
                  header: t("kubernetesHelm.operations"),
                  render: (r) => (
                    <div className="argus-k8s-workbench-controls">
                      <Button onPress={() => setSelected(r)}>
                        {t("kubernetesHelm.history")}
                      </Button>
                      {canManage && (
                        <>
                          <Button
                            isDisabled={
                              r.status !== "deployed" && r.status !== "failed"
                            }
                            onPress={() =>
                              setMutation({ operation: "upgrade", release: r })
                            }
                          >
                            {t("kubernetesHelm.actions.upgrade")}
                          </Button>
                          <Button
                            isDisabled={r.status === "uninstalled"}
                            onPress={() =>
                              setMutation({
                                operation: "uninstall",
                                release: r,
                              })
                            }
                          >
                            {t("kubernetesHelm.actions.uninstall")}
                          </Button>
                        </>
                      )}
                    </div>
                  ),
                },
              ]}
            />
          ) : (
            !list.isFetching &&
            !list.error && (
              <EmptyState
                title={t("kubernetesHelm.empty")}
                description={t("kubernetesHelm.scopeHint")}
              />
            )
          ))}
        {selected && history.data && !history.error && (
          <>
            <h3>
              {t("kubernetesHelm.releaseHistory", { name: selected.name })}
            </h3>
            <DataTable
              data={history.data.items}
              getRowKey={(r) => String(r.revision)}
              columns={[
                { key: "revision", header: t("kubernetesHelm.revision") },
                {
                  key: "chart",
                  header: t("kubernetesHelm.chart"),
                  render: (r) => `${r.chart} ${r.chart_version}`,
                },
                {
                  key: "status",
                  header: t("kubernetesWorkbench.mutation.status"),
                  render: (r) =>
                    t(`kubernetesHelm.statuses.${r.status}`, {
                      defaultValue: r.status,
                    }),
                },
                {
                  key: "updated_at",
                  header: t("kubernetesWorkbench.observed"),
                  render: (r) =>
                    new Date(r.updated_at).toLocaleString(i18n.language),
                },
                {
                  key: "actions",
                  header: t("kubernetesHelm.operations"),
                  render: (r) =>
                    canManage && (
                      <Button
                        isDisabled={
                          r.revision >=
                            Math.max(
                              ...history.data!.items.map((v) => v.revision),
                            ) || r.status === "uninstalled"
                        }
                        onPress={() =>
                          setMutation({
                            operation: "rollback",
                            release: history.data!.items.reduce((a, b) =>
                              a.revision > b.revision ? a : b,
                            ),
                            revision: r.revision,
                          })
                        }
                      >
                        {t("kubernetesHelm.actions.rollback")}
                      </Button>
                    ),
                },
              ]}
            />
          </>
        )}
      </Dialog>
      {picker && (
        <NamespacePicker
          clusterId={clusterId}
          onSelect={(n) => {
            setNamespace(n);
            setSelected(undefined);
            setPicker(false);
          }}
          onClose={() => setPicker(false)}
        />
      )}
    </>
  );
}
