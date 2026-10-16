import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { QueryBoundary } from "@argus/ui";
import {
  useApi,
  type KubernetesCluster,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Button,
  ConfirmDialog,
  EmptyState,
  FormDrawer,
  PageShell,
  Spinner,
} from "@argus/ui";
import { ClusterList } from "../components/kubernetes/cluster-list";
import "../styles/resource-lists.css";
import {
  ClusterFormDrawer,
  type ClusterFormState,
} from "../components/kubernetes/cluster-form-drawer";
import { PendingActionCard } from "../components/kubernetes/pending-action-card";
import "../styles/kubernetes.css";

/** Kubernetes 集群列表与真实接入、采集管理。 */
export function KubernetesPage() {
  const { t } = useTranslation();
  const api = useApi();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [formState, setFormState] = useState<ClusterFormState | null>(null);
  const [deleting, setDeleting] = useState<KubernetesCluster | null>(null);
  const [deleteAction, setDeleteAction] = useState<PendingActionPublic | null>(
    null,
  );

  const clustersQuery = useQuery({
    queryKey: ["kubernetes", "clusters"],
    queryFn: () => api.kubernetes.listClusters(),
    refetchInterval: 15_000,
  });

  const deleteCluster = useMutation({
    mutationFn: (cluster: KubernetesCluster) =>
      api.kubernetes.previewDeleteResource(
        cluster.id,
        cluster.resource_version,
      ),
    onSuccess: (action) => {
      setDeleteAction(action);
      setDeleting(null);
    },
  });

  const clusters = clustersQuery.data?.items ?? [];

  return (
    <PageShell
      actions={
        <Button
          onPress={() => setFormState({ mode: "create" })}
          variant="primary"
        >
          {t("kubernetes.addCluster")}
        </Button>
      }
      description={t("kubernetes.subtitle")}
      title={t("kubernetes.title")}
    >
      <QueryBoundary query={clustersQuery}>
        {clustersQuery.isLoading ? (
          <Spinner label={t("common.loading")} />
        ) : clustersQuery.isError ? (
          <EmptyState
            description={t("kubernetes.loadFailed")}
            kind="error"
            title={t("kubernetes.title")}
          />
        ) : clusters.length === 0 ? (
          <EmptyState
            action={
              <Button
                onPress={() => setFormState({ mode: "create" })}
                variant="primary"
              >
                {t("kubernetes.addCluster")}
              </Button>
            }
            description={t("kubernetes.empty.description")}
            title={t("kubernetes.empty.title")}
          />
        ) : (
          <ClusterList
            clusters={clusters}
            onDelete={setDeleting}
            onEdit={(cluster) => setFormState({ mode: "edit", cluster })}
            onOpenCollector={(cluster) =>
              void navigate({
                to: "/kubernetes/$clusterId",
                params: { clusterId: cluster.id },
                hash: "otlp-collector",
              })
            }
          />
        )}
      </QueryBoundary>
      <ClusterFormDrawer onClose={() => setFormState(null)} state={formState} />

      <ConfirmDialog
        confirmLabel={t("kubernetes.deleteDialog.confirm")}
        danger
        description={t("kubernetes.deleteDialog.description")}
        loading={deleteCluster.isPending}
        onConfirm={() => deleting && deleteCluster.mutate(deleting)}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        open={deleting !== null}
        title={`${t("kubernetes.deleteDialog.title")} · ${deleting?.name ?? ""}`}
      />

      <FormDrawer
        footer={<></>}
        onOpenChange={(open) => {
          if (!open) setDeleteAction(null);
        }}
        open={deleteAction !== null}
        title={t("kubernetes.deleteDialog.title")}
      >
        {deleteAction && (
          <PendingActionCard
            action={deleteAction}
            onDismiss={() => setDeleteAction(null)}
            onSettled={() => {
              setDeleteAction(null);
              void queryClient.invalidateQueries({ queryKey: ["kubernetes"] });
            }}
          />
        )}
      </FormDrawer>
    </PageShell>
  );
}
