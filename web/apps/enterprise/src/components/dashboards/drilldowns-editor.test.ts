import { expect, it } from "vitest";
import { emptyDashboardSpec } from "@argus/api-client";
import { newPanel } from "./model";
import { removeDrilldownGraph } from "./drilldowns-editor";

it("removes dependent cycles but preserves shared details and unrelated draft work", () => {
  const panel = newPanel(emptyDashboardSpec(), "Test");
  panel.detail_query_targets = ["a", "b", "unrelated"].map((id) => ({
    ...structuredClone(panel.targets[0]!),
    id,
  }));
  panel.drilldowns = [
    {
      id: "root",
      origin_query_ref: "main",
      detail_query_ref: "a",
      scope_policy: "inherit",
      inputs: {},
      title: "",
    },
    {
      id: "child",
      origin_query_ref: "a",
      detail_query_ref: "b",
      scope_policy: "inherit",
      inputs: {},
      title: "",
    },
    {
      id: "cycle",
      origin_query_ref: "b",
      detail_query_ref: "a",
      scope_policy: "inherit",
      inputs: {},
      title: "",
    },
  ];
  const removed = removeDrilldownGraph(panel, "root");
  expect(removed.drilldowns).toEqual([]);
  expect(removed.detail_query_targets.map((t) => t.id)).toEqual(["unrelated"]);
  panel.drilldowns.push({
    id: "shared",
    origin_query_ref: "main",
    detail_query_ref: "b",
    scope_policy: "inherit",
    inputs: {},
    title: "",
  });
  const shared = removeDrilldownGraph(panel, "root");
  expect(shared.drilldowns.map((d) => d.id)).toEqual([
    "child",
    "cycle",
    "shared",
  ]);
  expect(shared.detail_query_targets).toHaveLength(3);
  expect(panel.drilldowns).toHaveLength(4);
});
