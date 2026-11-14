import { formatResourceQuantity } from "@argus/ui";
import type { KubernetesInventoryObject } from "@argus/api-client";
import type { ComponentProps } from "react";
import type { Badge } from "@argus/ui";
type BadgeTone = NonNullable<ComponentProps<typeof Badge>["tone"]>;

export const resourceHoverFields: Record<string, string[]> = {
  pod: ["ready", "restarts", "node", "owner"],
  deployment: ["available", "ready", "updated", "images", "strategy"],
  statefulset: ["ready", "updated", "images", "strategy"],
  daemonset: ["ready", "available", "updated", "images", "strategy"],
  replicaset: ["ready", "images", "owner"],
  controllerrevision: ["revision", "owner"],
  job: ["active", "succeeded", "failed", "duration_seconds", "owner"],
  cronjob: ["schedule", "timezone", "active", "last_schedule"],
  pvc: ["requested_capacity", "actual_capacity", "storage_class", "volume"],
  pv: ["capacity", "storage_class", "claim", "reclaim_policy"],
  storageclass: ["provisioner", "binding_mode", "expandable", "reclaim_policy"],
  service: ["service_type", "cluster_ip", "ports", "selector"],
  ingress: ["ingress_class", "hosts", "paths", "addresses"],
  ingressclass: ["controller"],
  hpa: [
    "current_replicas",
    "desired_replicas",
    "min_replicas",
    "max_replicas",
    "target",
    "metric_current",
    "metric_targets",
  ],
  pdb: ["healthy", "desired_healthy", "disruptions_allowed", "selector"],
  node: [
    "roles",
    "scheduling",
    "cpu_allocatable",
    "memory_allocatable",
    "kubelet_version",
  ],
  configmap: ["keys"],
  secret: ["configuration_type", "keys"],
  namespace: ["phase"],
  networkpolicy: ["ingress_rules", "egress_rules"],
  role: ["rules", "rule_summary"],
  clusterrole: ["rules", "rule_summary"],
  rolebinding: ["role", "subject_names"],
  clusterrolebinding: ["role", "subject_names"],
  serviceaccount: ["automount_token"],
  resourcequota: ["quota"],
  limitrange: ["limits"],
  endpointslice: ["address_type", "ready_endpoints", "unknown_endpoints"],
  event: ["event_type", "event_reason", "message", "event_time"],
  crd: ["group", "kind", "scope"],
};
export const resourceColumns: Record<string, string[]> = {
  ...resourceHoverFields,
  pod: ["ready"],
  deployment: ["available"],
  statefulset: ["ready"],
  daemonset: ["available"],
  replicaset: ["ready"],
  namespace: [],
  node: ["scheduling"],
};
export function stateTone(state: string): BadgeTone {
  if (state === "healthy") return "success";
  if (state === "failed") return "danger";
  if (["pending", "warning"].includes(state)) return "warning";
  if (state === "progressing") return "accent";
  return "neutral";
}
export function objectRecord(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}
export function objectValue(
  object: KubernetesInventoryObject,
  path: string,
): unknown {
  let value: unknown = object.data;
  for (const key of path.split(".")) value = objectRecord(value)[key];
  return value;
}
export const readonlyKinds = new Set([
  "pv",
  "storageclass",
  "event",
  "endpointslice",
]);
export function referenceOf(
  row: KubernetesInventoryObject,
  namespaceId?: string,
) {
  return {
    type: row.definition.key,
    name: row.name,
    uid: row.uid,
    namespace: row.namespace,
    namespace_uid: row.namespace_uid,
    namespace_id: namespaceId,
  };
}

export function formatResourceField(key: string, value: string): string {
  if (value === "") return "—";
  if (key === "cpu_allocatable") return formatResourceQuantity(value, "cpu");
  if (key === "memory_allocatable")
    return formatResourceQuantity(value, "bytes");
  if (key === "duration_seconds") return value + " s";
  return value;
}
