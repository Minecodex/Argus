import type { BaseContext } from "./context";
import type {
  KubernetesInventoryObject,
  KubernetesResourceSnapshot,
} from "../kubernetes-workbench";
import { ensureKubernetesInventory } from "./kubernetes-seed";
import { dashboardObjectGrant, dashboardFailure } from "./dashboard-state";
import { resolvePermissions } from "./permissions";
const record = (v: unknown): Record<string, unknown> =>
  v !== null && typeof v === "object" && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
export function mockResourceView(
  row: KubernetesInventoryObject,
): KubernetesInventoryObject {
  const out = structuredClone(row),
    spec = record(row.data.spec),
    status = record(row.data.status);
  const fields: Record<string, string> = {};
  if (row.owners.length)
    fields.owner = row.owners
      .map((owner) => owner.kind + "/" + owner.name)
      .join(", ");
  let state: NonNullable<KubernetesInventoryObject["summary"]>["state"] =
    "unknown";
  for (const [key, value] of Object.entries(status))
    if (["string", "number"].includes(typeof value))
      fields[key] = String(value);
  if (row.definition.key === "pod") {
    fields.phase = String(status.phase ?? "");
    if (status.phase === "Running") state = "warning";
    if (status.phase === "Succeeded") state = "completed";
    if (status.phase === "Failed") state = "failed";
    if (status.phase === "Pending") state = "pending";
    if (
      Array.isArray(status.conditions) &&
      status.conditions.some(
        (c) => record(c).type === "Ready" && record(c).status === "True",
      )
    )
      state = "healthy";
    if (spec.nodeName) fields.node = String(spec.nodeName);
  }
  if (
    ["deployment", "statefulset", "daemonset", "replicaset"].includes(
      row.definition.key,
    )
  ) {
    fields.ready =
      String(status.readyReplicas ?? status.numberReady ?? "—") +
      " / " +
      String(spec.replicas ?? status.desiredNumberScheduled ?? "—");
    state = status.readyReplicas !== undefined ? "progressing" : "unknown";
  }
  if (row.definition.key === "cronjob") {
    fields.schedule = String(spec.schedule ?? "");
    state = spec.suspend ? "paused" : "configured";
  }
  if (row.definition.key === "pvc") {
    fields.requested_capacity = String(
      record(record(spec.resources).requests).storage ?? "",
    );
    fields.actual_capacity = String(record(status.capacity).storage ?? "");
    fields.storage_class = String(spec.storageClassName ?? "");
    state =
      status.phase === "Bound"
        ? "healthy"
        : status.phase === "Lost"
          ? "failed"
          : "pending";
  }
  if (
    [
      "service",
      "ingress",
      "configmap",
      "secret",
      "role",
      "clusterrole",
      "rolebinding",
      "clusterrolebinding",
      "namespace",
      "storageclass",
      "networkpolicy",
    ].includes(row.definition.key)
  )
    state = "configured";
  out.summary = { state, fields };
  const actions: string[] = [
    "pv",
    "storageclass",
    "event",
    "endpointslice",
  ].includes(row.definition.key)
    ? []
    : ["edit", "delete"];
  if (["deployment", "statefulset", "replicaset"].includes(row.definition.key))
    actions.push("scale");
  if (["deployment", "statefulset", "daemonset"].includes(row.definition.key))
    actions.push("configure", "image", "restart", "rollback");
  if (row.definition.key === "pod") actions.push("logs", "exec");
  if (row.definition.key === "cronjob")
    actions.push("run", "schedule", spec.suspend ? "resume" : "suspend");
  if (row.definition.key === "job")
    actions.push(spec.suspend ? "resume" : "suspend");
  if (row.definition.key === "pvc") actions.push("expand");
  if (row.definition.key === "node")
    actions.push(spec.unschedulable ? "uncordon" : "cordon", "drain");
  if (
    [
      "configmap",
      "secret",
      "hpa",
      "pdb",
      "service",
      "ingress",
      "networkpolicy",
      "role",
      "clusterrole",
      "rolebinding",
      "clusterrolebinding",
      "resourcequota",
      "limitrange",
      "namespace",
      "serviceaccount",
      "ingressclass",
    ].includes(row.definition.key)
  )
    actions.push("configure");
  out.actions = actions.map((action) => ({ action, state: "requires_check" }));
  return out;
}
export function mockResourceSnapshot(
  ctx: BaseContext,
  clusterId: string,
  namespaceIds: string[] = [],
): KubernetesResourceSnapshot {
  const permissions = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  if (!permissions.has("*") && !permissions.has("kubernetes.read"))
    dashboardFailure("DASHBOARD_DENIED", 403);
  if (
    !ctx.db.clusters.some(
      (row) => row.id === clusterId && row.enterpriseId === ctx.enterpriseId(),
    )
  )
    dashboardFailure("DASHBOARD_DENIED", 403);
  const data = ensureKubernetesInventory(ctx, clusterId),
    wide = dashboardObjectGrant(ctx, "kubernetes_cluster", clusterId);
  const allowed = data.namespaces.filter(
    (ns) => wide || dashboardObjectGrant(ctx, "kubernetes_namespace", ns.id),
  );
  if (!wide && !allowed.length) dashboardFailure("DASHBOARD_DENIED", 403);
  if (namespaceIds.some((id) => !allowed.some((ns) => ns.id === id)))
    dashboardFailure("DASHBOARD_DENIED", 403);
  const chosen = namespaceIds.length
    ? allowed.filter((ns) => namespaceIds.includes(ns.id))
    : allowed;
  const rows = data.objects
    .filter((row) =>
      row.definition.namespaced
        ? chosen.some((ns) => ns.uid === row.namespace_uid)
        : wide,
    )
    .map(mockResourceView);
  const counts: Record<string, Record<string, number>> = {};
  const resources = rows.map((row) => {
    const type = row.definition.key,
      summary = row.summary!;
    counts[type] ??= { total: 0 };
    counts[type]!.total = (counts[type]!.total ?? 0) + 1;
    counts[type]![summary.state] = (counts[type]![summary.state] ?? 0) + 1;
    return {
      type,
      uid: row.uid,
      name: row.name,
      namespace: row.namespace,
      namespace_uid: row.namespace_uid,
      namespace_id: chosen.find((ns) => ns.uid === row.namespace_uid)?.id,
      summary,
      actions: row.actions ?? [],
      created_at: row.created_at,
    };
  });
  const ids = new Set(resources.map((row) => row.uid));
  const relations = rows.flatMap((row) =>
    row.owners
      .filter((owner) => ids.has(owner.uid))
      .map((owner) => ({
        from: owner.uid,
        to: row.uid,
        kind: "owns" as const,
      })),
  );
  return {
    cluster_id: clusterId,
    cluster_wide: wide,
    resources,
    relations,
    counts,
    complete: true,
    errors: {},
    observed_at: ctx.nowIso(),
    scope_signature:
      "mock:" + clusterId + ":" + chosen.map((ns) => ns.uid).join(","),
  };
}
