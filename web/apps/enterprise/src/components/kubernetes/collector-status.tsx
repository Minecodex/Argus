import { BuiltinCollections } from "../monitoring/builtin-collections";
import { activeCollectorProfileIds } from "../telemetry/collector-profiles";
import { collectorReplicaStatus } from "./collector-replicas";
import { kubernetesCollectorNamespace } from "@argus/api-client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  useApi,
  type CollectorInstance,
  type KubernetesCluster,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  Field,
  Input,
  KeyValueGrid,
  Progress,
  StatCard,
  StatusBadge,
} from "@argus/ui";
import { PendingActionCard } from "./pending-action-card";
import { bindingCoverage, collectorStatusTone } from "./status";

/** 已安装 OTLP 收集器：状态面板（副本/覆盖率/冲突/出口）+ Profile 开关配置。 */
export function CollectorStatusPanel({
  cluster,
  collector,
}: {
  cluster: KubernetesCluster;
  collector: CollectorInstance;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const queryClient = useQueryClient();

  const bindingsQuery = useQuery({
    queryKey: ["kubernetes", "nodeBindings", cluster.id],
    queryFn: () => api.kubernetes.listNodeBindings(cluster.id),
  });
  const claimsQuery = useQuery({
    queryKey: ["kubernetes", "collectionClaims", cluster.id],
    queryFn: () => api.kubernetes.listCollectionClaims(cluster.id),
  });
  const workloadQuery = useQuery({
    queryKey: ["kubernetes", "collector-workload", cluster.id, collector.id],
    queryFn: () =>
      api.kubernetes.listResources(cluster.id, {
        resource_type: "daemonset",
        namespace: kubernetesCollectorNamespace,
        limit: 200,
      }),
    refetchInterval: 15_000,
  });

  const distributionsQuery = useQuery({
    queryKey: ["telemetry", "distributions"],
    queryFn: () => api.telemetry.listDistributions(),
  });

  const [pendingAction, setPendingAction] =
    useState<PendingActionPublic | null>(null);
  const [routeTested, setRouteTested] = useState(false);
  const [customImage, setCustomImage] = useState("");
  const [imagePullSecretInput, setImagePullSecretInput] = useState("");

  // 镜像按次输入:configure/upgrade/repair 都会生成携带镜像的 plan,
  // 留空回退服务端默认;与安装向导共用同一套格式规则。
  const trimmedImage = customImage.trim();
  const imageInvalid = trimmedImage !== "" && !/^\S+:\S+$/.test(trimmedImage);
  const imageOverride =
    trimmedImage && !imageInvalid ? { kubernetes_image: trimmedImage } : {};
  const imagePullSecrets = imagePullSecretInput
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
  const imagePullSecretsInvalid =
    imagePullSecrets.length > 16 ||
    new Set(imagePullSecrets).size !== imagePullSecrets.length ||
    imagePullSecrets.some(
      (name) =>
        name.length > 253 || !/^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$/.test(name),
    );
  const imagePullSecretOverride = imagePullSecrets.length
    ? { image_pull_secrets: imagePullSecrets }
    : {};

  const changed = trimmedImage !== "" || imagePullSecretInput.trim() !== "";
  const previewChange = useMutation({
    mutationFn: () => {
      if (!collector.route) {
        throw new Error("COLLECTOR_ROUTE_TRANSPORT_INVALID");
      }
      const profileIds = activeCollectorProfileIds(
        claimsQuery.data ?? [],
        collector.id,
      );

      return api.kubernetes.previewCollectorAction(cluster.id, "configure", {
        distribution_version_id: collector.distribution_version_id,
        profile_ids: profileIds,
        route_kind: collector.route.kind,
        transport: collector.route.transport,
        loopback_port: collector.route.loopback_port,
        gateway_collector_id: collector.route.gateway_collector_id,
        expected_version: collector.version,
        ...imageOverride,
        ...imagePullSecretOverride,
      });
    },
    onSuccess: setPendingAction,
  });

  const previewLifecycle = useMutation({
    mutationFn: (action: "upgrade" | "repair" | "uninstall") => {
      if (!collector.route) {
        throw new Error("COLLECTOR_ROUTE_TRANSPORT_INVALID");
      }
      const profileIds = [
        ...new Set(
          (claimsQuery.data ?? [])
            .filter(
              (claim) =>
                claim.collector_id === collector.id &&
                claim.status === "active",
            )
            .map((claim) => claim.profile_id)
            .filter((id): id is string => Boolean(id)),
        ),
      ];
      return api.kubernetes.previewCollectorAction(cluster.id, action, {
        distribution_version_id: collector.distribution_version_id,
        profile_ids: profileIds,
        route_kind: collector.route.kind,
        transport: collector.route.transport,
        loopback_port: collector.route.loopback_port,
        gateway_collector_id: collector.route.gateway_collector_id,
        expected_version: collector.version,
        ...(action === "uninstall"
          ? {}
          : { ...imageOverride, ...imagePullSecretOverride }),
      });
    },
    onSuccess: setPendingAction,
  });

  const testRoute = useMutation({
    mutationFn: () => {
      if (!collector.route) {
        throw new Error("COLLECTOR_ROUTE_TRANSPORT_INVALID");
      }
      return api.telemetry.testRoute({
        collector_id: collector.id,
        route_kind: collector.route.kind,
        transport: collector.route.transport,
        gateway_collector_id: collector.route.gateway_collector_id,
      });
    },
    onSuccess: (result) => setRouteTested(result.status === "succeeded"),
  });

  const coverage =
    bindingsQuery.isSuccess && cluster.node_count !== undefined
      ? bindingCoverage(cluster, bindingsQuery.data)
      : undefined;
  const replicas = workloadQuery.isSuccess
    ? collectorReplicaStatus(workloadQuery.data.items, collector.id)
    : undefined;
  const unavailable = t("kubernetes.card.dataUnavailable");
  const distribution = distributionsQuery.data?.find(
    (entry) => entry.id === collector.distribution_version_id,
  );
  const conflicts = (claimsQuery.data ?? []).filter(
    (claim) => claim.status === "conflict",
  );
  const egressOk = cluster.connection_status === "connected";
  const defaultImage =
    distributionsQuery.data?.find(
      (item) =>
        item.support_status === "supported" &&
        item.artifacts.some((artifact) => artifact.platform === "linux_arm64"),
    )?.kubernetes_image ?? "—";

  return (
    <div className="argus-k8s-stack">
      <div className="argus-k8s-stat-grid">
        <StatCard
          label={t("kubernetes.collector.status.state")}
          value={
            <StatusBadge tone={collectorStatusTone(collector.status)}>
              {t(`kubernetes.collectorStatus.${collector.status}`)}
            </StatusBadge>
          }
        />
        <StatCard
          detail={
            coverage ? `${coverage.verified}/${coverage.total}` : unavailable
          }
          label={t("kubernetes.collector.status.bindingCoverage")}
          tone={coverage && coverage.percent >= 100 ? "success" : "warning"}
          value={coverage ? `${coverage.percent}%` : unavailable}
        />
        <StatCard
          label={t("kubernetes.collector.status.claimConflicts")}
          tone={conflicts.length > 0 ? "danger" : "success"}
          value={
            !claimsQuery.isSuccess
              ? unavailable
              : conflicts.length > 0
                ? String(conflicts.length)
                : t("kubernetes.collector.status.noConflicts")
          }
        />
        <StatCard
          label={t("kubernetes.collector.status.version")}
          value={distribution?.collector_version || unavailable}
        />
      </div>

      <Card>
        <CardHeader title={t("kubernetes.collector.status.readyReplicas")} />
        <CardContent>
          {replicas ? (
            <Progress
              label={`${replicas.ready}/${replicas.desired} (${replicas.percent}%)`}
              tone={replicas.ready === replicas.desired ? "success" : "accent"}
              value={replicas.percent}
            />
          ) : (
            <p className="argus-muted">
              {workloadQuery.isPending ? t("common.loading") : unavailable}
            </p>
          )}
          {collector.status === "degraded" &&
            collector.last_operation_error_code && (
              <p className="argus-mono">
                {t("hosts.components.lastFailure", {
                  code: collector.last_operation_error_code,
                })}
              </p>
            )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader title={t("kubernetes.collector.status.lifecycleTitle")} />
        <CardContent>
          {pendingAction ? (
            <PendingActionCard
              action={pendingAction}
              onSettled={() => {
                setPendingAction(null);
                void queryClient.invalidateQueries({
                  queryKey: ["kubernetes"],
                });
              }}
            />
          ) : (
            <div className="argus-form-actions">
              <Button
                isPending={testRoute.isPending}
                onPress={() => testRoute.mutate()}
                variant="secondary"
              >
                {t("kubernetes.collector.status.testRoute")}
              </Button>
              {routeTested && (
                <StatusBadge tone="success">
                  {t("kubernetes.collector.status.routeOk")}
                </StatusBadge>
              )}
              <Button
                isDisabled={imageInvalid || imagePullSecretsInvalid}
                isPending={previewLifecycle.isPending}
                onPress={() => previewLifecycle.mutate("upgrade")}
                variant="secondary"
              >
                {t("kubernetes.collector.status.upgrade")}
              </Button>
              <Button
                isDisabled={imageInvalid || imagePullSecretsInvalid}
                isPending={previewLifecycle.isPending}
                onPress={() => previewLifecycle.mutate("repair")}
                variant="secondary"
              >
                {t("kubernetes.collector.status.repair")}
              </Button>
              <Button
                isPending={previewLifecycle.isPending}
                onPress={() => previewLifecycle.mutate("uninstall")}
                variant="danger"
              >
                {t("kubernetes.collector.status.uninstall")}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader title={t("kubernetes.detail.collectorTab")} />
        <CardContent>
          <Field
            label={t("kubernetes.collector.wizard.imageAddress")}
            hint={t("kubernetes.collector.wizard.imageAddressDesc")}
            requirement="optional"
            error={
              imageInvalid
                ? t("kubernetes.collector.wizard.imageInvalid")
                : undefined
            }
          >
            <Input
              className="argus-mono"
              onChange={(event) => setCustomImage(event.target.value)}
              placeholder={
                defaultImage === "—"
                  ? "registry.example.com/argus-otelcol:0.1.0-m7"
                  : defaultImage
              }
              value={customImage}
            />
          </Field>
          <Field
            label={t("kubernetes.collector.wizard.imagePullSecrets")}
            hint={t("kubernetes.collector.wizard.imagePullSecretsDesc")}
            requirement="optional"
            error={
              imagePullSecretsInvalid
                ? t("kubernetes.collector.wizard.imagePullSecretsInvalid")
                : undefined
            }
          >
            <Input
              className="argus-mono"
              onChange={(event) => setImagePullSecretInput(event.target.value)}
              placeholder={t(
                "kubernetes.collector.wizard.imagePullSecretsPlaceholder",
              )}
              value={imagePullSecretInput}
            />
          </Field>
          <KeyValueGrid
            columns={2}
            items={[
              {
                label: t("kubernetes.collector.status.revision"),
                value: `${collector.effective_revision} → ${collector.desired_revision}`,
              },
              {
                label: t("kubernetes.collector.wizard.profile"),
                value: collector.distribution_version_id,
              },
              {
                label: t("kubernetes.collector.status.imageDigest"),
                value: (
                  <span className="argus-mono">
                    {trimmedImage || defaultImage}
                  </span>
                ),
              },
              {
                label: t("kubernetes.collector.status.egress"),
                value: (
                  <StatusBadge tone={egressOk ? "success" : "warning"}>
                    {egressOk
                      ? t("kubernetes.collector.status.egressOk")
                      : t("kubernetes.collector.status.egressDegraded")}
                  </StatusBadge>
                ),
              },
            ]}
          />
        </CardContent>
      </Card>

      {conflicts.length > 0 && (
        <Card>
          <CardHeader title={t("kubernetes.collector.status.claimConflicts")} />
          <CardContent>
            <ul className="argus-k8s-events">
              {conflicts.map((claim) => (
                <li key={claim.id}>
                  <StatusBadge tone="warning">
                    {t("kubernetes.claimState.closeHostProfile")}
                  </StatusBadge>
                  <span>
                    {claim.profile_id ?? claim.claim_type} · {claim.signal} ·{" "}
                    {claim.physical_resource_ref}
                  </span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader
          description={t("builtinCollections.clusterHint")}
          title={t("builtinCollections.clusterTitle")}
        />
        <CardContent>
          <div className="argus-k8s-stack">
            <BuiltinCollections kind="kubernetes_cluster" id={cluster.id} />

            {pendingAction ? (
              <PendingActionCard
                action={pendingAction}
                onSettled={() => {
                  setPendingAction(null);

                  void queryClient.invalidateQueries({
                    queryKey: ["kubernetes"],
                  });
                }}
              />
            ) : (
              <div>
                <Button
                  isDisabled={
                    !changed || imageInvalid || imagePullSecretsInvalid
                  }
                  isPending={previewChange.isPending}
                  onPress={() => previewChange.mutate()}
                  variant="primary"
                >
                  {t("kubernetes.collector.status.previewChange")}
                </Button>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
