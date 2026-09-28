import { expect, it } from "vitest";
import type { DashboardSchemas } from "@argus/api-client";
import { installationLabel, sourceValueLabels } from "./source-summary";

it("keeps unknown source metadata distinct from historical installation", () => {
  const source: DashboardSchemas["DashboardResolvedSource"] = {
    id: "s",
    resource_id: "r",
    generation: "g",
    revision: 1,
    type: "otlp",
    capability_version: "v1",
  };
  const t = (key: string) => key;
  expect(installationLabel(source, t)).toBe("dashboards.unknownInstallation");
  expect(installationLabel({ ...source, current_installation: false }, t)).toBe(
    "dashboards.historicalInstallation",
  );
  expect(
    sourceValueLabels(
      [
        { ...source, current_installation: true },
        { ...source, revision: 2, current_installation: true },
      ],
      t,
    ),
  ).toEqual({ s: "dashboards.currentInstallation · s" });
});
