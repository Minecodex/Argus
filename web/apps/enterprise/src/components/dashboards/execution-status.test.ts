import { expect, it } from "vitest";
import { emptyDashboardSpec, type DashboardExecution } from "@argus/api-client";
import { newPanel } from "./model";
import { panelCandidateResources } from "./execution-status";

it("limits local candidates to resolved resources applicable to the panel", () => {
  const panel = newPanel(emptyDashboardSpec(), "Cluster metrics");
  panel.applicable_resource_types = ["kubernetes_cluster"];
  const execution = {
    resources: [
      { id: "host", type: "host" },
      { id: "cluster", type: "kubernetes_cluster" },
    ],
  } as DashboardExecution;
  expect(panelCandidateResources(execution, panel)).toEqual(["cluster"]);
  expect(
    panelCandidateResources(
      { ...execution, resources: [execution.resources[0]!] },
      panel,
    ),
  ).toEqual([]);
  expect(panelCandidateResources(undefined, panel)).toEqual([]);
});
