import { describe, expect, it } from "vitest";
import type { CollectionClaim, CollectionProfile } from "@argus/api-client";
import {
  clusterProfileBaseline,
  clusterProfileIds,
} from "./collector-profile-draft";

const catalog = [
  { id: "nodes", key: "k8s-node-container" },
  { id: "cluster", key: "k8s-cluster" },
  { id: "gateway", key: "k8s-otlp-gateway" },
] as CollectionProfile[];
describe("cluster collection profile drafts", () => {
  it("uses this Collector's persisted Claims and ignores released, expired and foreign Claims", () => {
    const claims = [
      { collector_id: "target", profile_id: "nodes", status: "conflict" },
      { collector_id: "target", profile_id: "cluster", status: "released" },
      { collector_id: "target", profile_id: "gateway", status: "expired" },
      { collector_id: "other", profile_id: "gateway", status: "active" },
    ] as CollectionClaim[];
    expect(clusterProfileBaseline("target", claims, catalog)).toEqual({
      nodeContainer: true,
      clusterMetrics: false,
      otlpGateway: false,
    });
  });
  it("does not re-enable the node profile after its switch is turned off", () => {
    expect(
      clusterProfileIds(
        { nodeContainer: false, clusterMetrics: true, otlpGateway: true },
        catalog,
      ),
    ).toEqual(["cluster", "gateway"]);
    expect(
      clusterProfileIds(
        { nodeContainer: false, clusterMetrics: false, otlpGateway: false },
        catalog,
      ),
    ).toEqual([]);
  });
});
