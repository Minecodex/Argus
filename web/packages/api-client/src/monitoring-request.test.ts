import { describe, expect, it } from "vitest";
import type { MonitoringMutation } from "./monitoring";
import { monitoringRequest } from "./monitoring-request";

describe("monitoring request boundary", () => {
  it("projects observed workload data into writable identity without mutating the read model", () => {
    const observed = {
      namespace_id: "namespace-id",
      type: "deployment" as const,
      name: "orders",
      uid: "deployment-uid",
      containers: ["http"],
      namespace: "observed-name",
      namespace_uid: "server-attested-uid",
      ancestors: ["observed-owner"],
    };
    const input: MonitoringMutation = {
      kind: "collection",
      operation: "update",
      collection: {
        integration: "k8slog",
        enabled: true,
        config: { scope: "business", workload: observed },
      },
    };
    const request = monitoringRequest(input);
    expect(request.collection?.config?.workload).toEqual({
      namespace_id: observed.namespace_id,
      type: observed.type,
      name: observed.name,
      uid: observed.uid,
      containers: observed.containers,
    });
    expect(observed.namespace_uid).toBe("server-attested-uid");
    expect(request.collection?.config?.workload?.containers).not.toBe(
      observed.containers,
    );
    expect(request.collection?.config?.scope).toBe("business");
  });

  it("keeps a dedicated enable/disable request minimal", () => {
    const input: MonitoringMutation = {
      kind: "collection",
      operation: "set_enabled",
      collection: { enabled: false },
    };
    expect(monitoringRequest(input)).toBe(input);
  });
});
