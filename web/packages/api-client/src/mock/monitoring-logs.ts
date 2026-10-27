import type {
  MonitoringCollection,
  MonitoringMutation,
  MonitoringSchemas,
} from "../monitoring";
import type { BaseContext } from "./context";
import type { MockMonitoringState } from "./monitoring";
import { dashboardObjectGrant } from "./dashboard-state";
import { kubernetesObjectContainers } from "../kubernetes-workbench";

export function canUseMonitoringCollector(
  ctx: BaseContext,
  resource: { resource_type: string; resource_id: string },
  namespaceId?: string,
) {
  if (dashboardObjectGrant(ctx, resource.resource_type, resource.resource_id))
    return true;
  return (
    resource.resource_type === "kubernetes_cluster" &&
    !!ctx.db.kubernetesInventory?.[resource.resource_id]?.namespaces.some(
      (ns) =>
        (!namespaceId || ns.id === namespaceId) &&
        dashboardObjectGrant(ctx, "kubernetes_namespace", ns.id),
    )
  );
}
export function canReadMonitoringCollection(
  ctx: BaseContext,
  row: MonitoringCollection,
) {
  if (dashboardObjectGrant(ctx, row.target.kind, row.target.id)) return true;
  const w = row.config.workload;
  if (
    row.integration !== "k8slog" ||
    row.target.kind !== "kubernetes_cluster" ||
    !w
  )
    return false;
  const ns = ctx.db.kubernetesInventory?.[row.target.id]?.namespaces.find(
    (ns) => ns.id === w.namespace_id && ns.uid === w.namespace_uid,
  );
  return !!ns && dashboardObjectGrant(ctx, "kubernetes_namespace", ns.id);
}

export const isLogRule = (key?: string) =>
  ["filelog", "journald", "eventlog", "k8slog"].includes(key ?? "");

export function logRuleTarget(
  ctx: BaseContext,
  input: NonNullable<MonitoringMutation["collection"]>,
): MonitoringSchemas["MonitoringCollectionResourceRef"] {
  if (input.config?.scope === "business")
    return { kind: "application", id: input.application_id! };
  const collector = ctx.db.collectors.find((v) => v.id === input.collector_id)!;
  return { kind: collector.resource_type, id: collector.resource_id };
}

export function validateLogRule(
  ctx: BaseContext,
  input: NonNullable<MonitoringMutation["collection"]>,
  data: MockMonitoringState,
  existing: MonitoringCollection | undefined,
  fail: (code?: string, status?: number) => never,
  permit: (permission: string) => void,
) {
  permit("telemetry.collector.manage");
  const collector = ctx.db.collectors.find((v) => v.id === input.collector_id);
  const kubernetes = input.integration === "k8slog";
  if (
    !collector ||
    collector.resource_type !== (kubernetes ? "kubernetes_cluster" : "host")
  )
    fail("MONITORING_UNSUPPORTED_INTEGRATION", 409);
  const rule: MonitoringSchemas["MonitoringCollectionConfig"] | undefined =
    input.config;
  if (
    !rule ||
    !["business", "system", "component"].includes(String(rule.scope)) ||
    input.target_identity ||
    input.target_name ||
    input.bindings?.length ||
    input.credential_update === "replace"
  )
    fail();
  if (rule.scope === "business") {
    permit("application.manage");
    if (
      !input.application_id ||
      !input.service_id ||
      !data.services.some(
        (v) =>
          v.id === input.service_id &&
          v.application_id === input.application_id &&
          v.status === "active",
      )
    )
      fail();
    if (
      !data.applications.some(
        (v) => v.id === input.application_id && v.status === "active",
      ) ||
      !dashboardObjectGrant(ctx, "application", input.application_id)
    )
      fail("MONITORING_DENIED", 403);
  } else if (input.application_id || input.service_id) fail();
  if (
    input.integration === "filelog" &&
    (!Array.isArray(rule.include) || !rule.include.length)
  )
    fail();
  const target = logRuleTarget(ctx, input);
  if (kubernetes) {
    const w = rule.workload;
    if (
      !w ||
      !w.containers.length ||
      !canUseMonitoringCollector(ctx, collector, w.namespace_id)
    )
      fail("MONITORING_DENIED", 403);
    const inventory = ctx.db.kubernetesInventory?.[collector.resource_id];
    const ns = inventory?.namespaces.find((ns) => ns.id === w.namespace_id);
    if (!ns) fail("MONITORING_DENIED", 403);
    if (!existing || input.enabled !== false) {
      const object = inventory?.objects.find(
        (row) =>
          row.definition.key === w.type &&
          row.uid === w.uid &&
          row.name === w.name &&
          row.namespace_uid === ns.uid,
      );
      if (
        !object ||
        w.containers.some(
          (name) =>
            !kubernetesObjectContainers(object).some(
              (row) => row.name === name,
            ),
        )
      )
        fail("MONITORING_VERSION_CONFLICT", 409);
    }
    rule.workload = {
      ...w,
      namespace: ns.name,
      namespace_uid: ns.uid,
      ancestors: [],
    };
  }
  if (
    existing &&
    (existing.target.id !== target.id || existing.target.kind !== target.kind)
  )
    fail();
  // Mock models exact duplicate rules; arbitrary glob intersections are enforced
  // by the server's bounded automata and its PostgreSQL concurrency tests.
  if (
    input.enabled &&
    data.collections?.some(
      (v) =>
        v.id !== existing?.id &&
        v.status === "active" &&
        v.desired_enabled &&
        v.collector_id === input.collector_id &&
        v.integration === input.integration &&
        (kubernetes
          ? v.config.workload?.namespace_id === rule.workload?.namespace_id &&
            v.config.workload?.uid === rule.workload?.uid &&
            v.config.workload?.containers.some((name) =>
              rule.workload?.containers.includes(name),
            )
          : JSON.stringify(v.config.include) === JSON.stringify(rule.include) &&
            JSON.stringify(v.config.exclude) === JSON.stringify(rule.exclude)),
    )
  )
    fail("MONITORING_LOG_RULE_OVERLAP", 409);
}
