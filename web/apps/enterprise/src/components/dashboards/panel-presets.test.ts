import { describe, expect, it } from "vitest";
import { emptyDashboardSpec } from "@argus/api-client";
import {
  createChartPanel,
  createPresetPanel,
  panelPresets,
} from "./panel-presets";
describe("panel scenario contracts", () => {
  it("keeps cumulative counters and histogram P95 queries type-safe", () => {
    for (const id of ["network", "disk", "requestRate", "latency"]) {
      const preset = panelPresets.find((p) => p.id === id)!;
      const panel = createPresetPanel(emptyDashboardSpec(), preset, id),
        target = panel.targets[0]!;
      expect(target.source_definition.dsl).toBeUndefined();
      expect(target.source_definition.builder!.window_seconds).toBe(300);
      expect(target.source_definition.builder!.metric_type).toBe(
        id === "latency" ? "histogram" : "counter",
      );
      expect(target.source_definition.builder!.operation).toBe(
        id === "latency" ? "p95" : "rate",
      );
    }
  });
  it("keeps APM on received trace samples and separates dedicated source requirements", () => {
    const panel = createPresetPanel(
      emptyDashboardSpec(),
      panelPresets.find((p) => p.id === "red")!,
      "RED",
    );
    expect(panel.signal).toBe("traces");
    expect(panel.targets[0]!.source_definition.builder!.operation).toBe(
      "apm_red",
    );
    expect(panel.targets[0]!.source_definition.builder!.bucket_seconds).toBe(
      60,
    );
    expect(
      panelPresets
        .filter((p) => p.requires === "kubelet")
        .every((p) => p.source === "kubeletstats"),
    ).toBe(true);
  });
  it("starting from histogram cannot retain the CPU metric", () => {
    const panel = createChartPanel(
      emptyDashboardSpec(),
      "histogram",
      "Histogram",
    );
    expect(panel.targets[0]!.source_definition.builder!.metric).toBe("");
    expect(panel.type).toBe("histogram");
  });
});
