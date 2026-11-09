import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type CollectorInstance,
  type PendingActionPublic,
} from "@argus/api-client";
import { Alert, Button, StatusBadge } from "@argus/ui";
import { activeCollectorProfileIds } from "../telemetry/collector-profiles";
import { PendingActionCard } from "./pending-action-card";
import { CollectorImageFields } from "./collector-image-fields";
import {
  collectorImageOptions,
  useCollectorImageForm,
} from "./collector-image-form";

/** Secondary administrative controls; collection choices stay in the main view. */
export function CollectorRuntimeSettings({
  clusterId,
  collector,
}: {
  clusterId: string;
  collector: CollectorInstance;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const cache = useQueryClient();
  const claims = useQuery({
    queryKey: ["kubernetes", "collectionClaims", clusterId],
    queryFn: () => api.kubernetes.listCollectionClaims(clusterId),
  });
  const distributions = useQuery({
    queryKey: ["telemetry", "distributions"],
    queryFn: () => api.telemetry.listDistributions(),
  });
  const form = useCollectorImageForm({
    image: t("kubernetes.collector.wizard.imageInvalid"),
    pullSecrets: t("kubernetes.collector.wizard.imagePullSecretsInvalid"),
  });
  const [image, pullSecrets] = form.watch(["image", "pullSecrets"]);
  const [pending, setPending] = useState<PendingActionPublic>();
  const preview = useMutation({
    mutationFn: (action: "configure" | "upgrade" | "repair" | "uninstall") => {
      if (!collector.route || !claims.data)
        throw new Error("COLLECTOR_ROUTE_TRANSPORT_INVALID");
      return api.kubernetes.previewCollectorAction(clusterId, action, {
        distribution_version_id: collector.distribution_version_id,
        profile_ids: activeCollectorProfileIds(claims.data, collector.id),
        route_kind: collector.route.kind,
        transport: collector.route.transport,
        loopback_port: collector.route.loopback_port,
        gateway_collector_id: collector.route.gateway_collector_id,
        expected_version: collector.version,
        ...(action === "uninstall"
          ? {}
          : collectorImageOptions(form.getValues())),
      });
    },
    onSuccess: setPending,
  });
  const route = useMutation({
    mutationFn: () => {
      if (!collector.route)
        throw new Error("COLLECTOR_ROUTE_TRANSPORT_INVALID");
      return api.telemetry.testRoute({
        collector_id: collector.id,
        route_kind: collector.route.kind,
        transport: collector.route.transport,
        gateway_collector_id: collector.route.gateway_collector_id,
      });
    },
  });
  const error = claims.error || preview.error || route.error;
  const disabled =
    claims.isPending ||
    !!claims.error ||
    Object.keys(form.formState.errors).length > 0 ||
    preview.isPending;
  const distribution = distributions.data?.find(
    (value) => value.id === collector.distribution_version_id,
  );
  if (pending)
    return (
      <PendingActionCard
        action={pending}
        onSettled={() => {
          setPending(undefined);
          void cache.invalidateQueries({ queryKey: ["kubernetes"] });
          void cache.invalidateQueries({
            queryKey: [
              "monitoring",
              "builtins",
              "kubernetes_cluster",
              clusterId,
            ],
          });
        }}
      />
    );
  return (
    <div className="argus-k8s-stack">
      <p className="argus-muted">{t("kubernetes.collector.settings.hint")}</p>
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
      <form
        className="argus-k8s-mutation-form"
        onSubmit={form.handleSubmit(() => preview.mutate("configure"))}
      >
        <CollectorImageFields
          form={form}
          defaultImage={distribution?.kubernetes_image}
        />
        <div className="argus-form-actions">
          <Button
            type="submit"
            variant="primary"
            isDisabled={disabled || !(image.trim() || pullSecrets.trim())}
            isPending={preview.isPending}
          >
            {t("kubernetes.collector.status.previewChange")}
          </Button>
        </div>
      </form>
      <div className="argus-form-actions">
        <Button onPress={() => route.mutate()} isPending={route.isPending}>
          {t("kubernetes.collector.status.testRoute")}
        </Button>
        {route.data?.status === "succeeded" && (
          <StatusBadge tone="success">
            {t("kubernetes.collector.status.routeOk")}
          </StatusBadge>
        )}
        <Button
          isDisabled={disabled}
          onPress={() =>
            void form.handleSubmit(() => preview.mutate("upgrade"))()
          }
        >
          {t("kubernetes.collector.status.upgrade")}
        </Button>
        <Button
          isDisabled={disabled}
          onPress={() =>
            void form.handleSubmit(() => preview.mutate("repair"))()
          }
        >
          {t("kubernetes.collector.status.repair")}
        </Button>
        <Button
          variant="danger"
          isDisabled={disabled}
          onPress={() => preview.mutate("uninstall")}
        >
          {t("kubernetes.collector.status.uninstall")}
        </Button>
      </div>
    </div>
  );
}
