import { expect, it } from "vitest";
import type { KubernetesResource } from "@argus/api-client";
import { collectorReplicaStatus } from "./collector-replicas";
const resource = (
  id: string,
  status: Record<string, unknown>,
): KubernetesResource => ({
  cluster_id: "cluster",
  resource_type: "daemonset",
  name: "collector",
  namespace: "argus-telemetry",
  labels: { "argus.io/collector-id": id },
  summary: { status },
});
it("reads actual ready nodes for the matching Collector instead of control-state percentages", () => {
  expect(
    collectorReplicaStatus(
      [
        resource("other", { desiredNumberScheduled: 4, numberReady: 4 }),
        resource("selected", { desiredNumberScheduled: 4, numberReady: 1 }),
      ],
      "selected",
    ),
  ).toEqual({ desired: 4, ready: 1, percent: 25 });
});
it("keeps missing and invalid workload observations unavailable", () => {
  expect(collectorReplicaStatus([], "selected")).toBeUndefined();
  expect(
    collectorReplicaStatus(
      [resource("selected", { desiredNumberScheduled: 2, numberReady: 3 })],
      "selected",
    ),
  ).toBeUndefined();
  expect(
    collectorReplicaStatus([resource("selected", {})], "selected"),
  ).toBeUndefined();
});
