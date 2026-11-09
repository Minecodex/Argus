import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type KubernetesCluster,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Alert,
  Button,
  Card,
  CardContent,
  CardHeader,
  Dialog,
} from "@argus/ui";
import { BuiltinCollections } from "../monitoring/builtin-collections";
import { CollectorSettlementPanel } from "../telemetry/collector-settlement";
import { PendingActionCard } from "./pending-action-card";
import { usePermission } from "../../lib/permissions";
import { CollectorImageFields } from "./collector-image-fields";
import {
  collectorImageOptions,
  useCollectorImageForm,
} from "./collector-image-form";

/** Show collection choices first, with a single installation entry. */
export function CollectorWizard({
  cluster,
  onInstalled,
}: {
  cluster: KubernetesCluster;
  onInstalled?: () => void;
}) {
  const { t } = useTranslation();
  const canManage = usePermission("telemetry.collector.manage");
  const [open, setOpen] = useState(false);
  return (
    <>
      <Card>
        <CardHeader
          title={t("builtinCollections.clusterTitle")}
          description={t("kubernetes.collector.notInstalledHint")}
          action={
            canManage && (
              <Button variant="primary" onPress={() => setOpen(true)}>
                {t("kubernetes.collector.startInstall")}
              </Button>
            )
          }
        />
        <CardContent>
          <BuiltinCollections
            kind="kubernetes_cluster"
            id={cluster.id}
            showTitle={false}
          />
        </CardContent>
      </Card>
      {open && (
        <Dialog
          open
          title={t("kubernetes.collector.startInstall")}
          onOpenChange={setOpen}
        >
          <CollectorInstallFlow
            cluster={cluster}
            onInstalled={() => {
              setOpen(false);
              onInstalled?.();
            }}
          />
        </Dialog>
      )}
    </>
  );
}

function CollectorInstallFlow({
  cluster,
  onInstalled,
}: {
  cluster: KubernetesCluster;
  onInstalled: () => void;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const cache = useQueryClient();
  const [advanced, setAdvanced] = useState(false);
  const form = useCollectorImageForm({
    image: t("kubernetes.collector.wizard.imageInvalid"),
    pullSecrets: t("kubernetes.collector.wizard.imagePullSecretsInvalid"),
  });
  const [pending, setPending] = useState<PendingActionPublic>();
  const [settling, setSettling] = useState(false);
  const distributions = useQuery({
    queryKey: ["telemetry", "distributions"],
    queryFn: () => api.telemetry.listDistributions(),
  });
  const distribution = distributions.data?.find(
    (value) =>
      value.support_status === "supported" &&
      value.artifacts.some(
        (artifact) =>
          artifact.platform === "linux_arm64" ||
          artifact.platform === "linux_amd64",
      ),
  );
  const install = useMutation({
    mutationFn: () => {
      if (!distribution)
        throw new Error(t("kubernetes.collector.wizard.catalogUnavailable"));
      return api.kubernetes.previewCollectorInstall(cluster.id, {
        distribution_version_id: distribution.id,
        profile_ids: [],
        route_kind: "direct_argus",
        transport: "direct",
        ...collectorImageOptions(form.getValues()),
      });
    },
    onSuccess: setPending,
  });
  const installed = () => {
    void cache.invalidateQueries({ queryKey: ["kubernetes"] });
    void cache.invalidateQueries({
      queryKey: ["monitoring", "builtins", "kubernetes_cluster", cluster.id],
    });
    onInstalled();
  };
  if (settling)
    return (
      <CollectorSettlementPanel
        onClose={installed}
        onSettled={installed}
        poll={() => api.kubernetes.getCollector(cluster.id)}
      />
    );
  if (pending)
    return (
      <PendingActionCard
        action={pending}
        onSettled={(confirmed) => {
          setPending(undefined);
          if (confirmed) setSettling(true);
        }}
      />
    );
  const error = install.error || distributions.error;
  return (
    <form
      className="argus-k8s-mutation-form"
      onSubmit={form.handleSubmit(() => {
        if (distribution) install.mutate();
      })}
    >
      <p>{t("kubernetes.collector.wizard.simpleHint")}</p>
      {error && (
        <Alert
          tone="danger"
          title={t("common.collectorSettlement.failed")}
          description={formatApiError(
            error,
            t("kubernetes.loadFailed"),
            (requestId) => t("common.requestReference", { requestId }),
          )}
        />
      )}
      <Button variant="ghost" onPress={() => setAdvanced((value) => !value)}>
        {t("kubernetes.collector.wizard.advanced")}
      </Button>
      {advanced && (
        <CollectorImageFields
          form={form}
          defaultImage={distribution?.kubernetes_image}
        />
      )}
      <div className="argus-form-actions">
        <Button
          type="submit"
          variant="primary"
          isPending={install.isPending}
          isDisabled={
            Object.keys(form.formState.errors).length > 0 || !distribution
          }
        >
          {t("kubernetes.collector.wizard.submit")}
        </Button>
      </div>
    </form>
  );
}
