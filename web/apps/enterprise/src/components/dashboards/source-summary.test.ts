import { expect, it } from "vitest";
import type { DashboardSchemas } from "@argus/api-client";
import {
  installationLabel,
  sourceRows,
  sourceValueLabels,
} from "./source-summary";

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

it("merges revisions of each source while retaining distinct installations", () => {
  const source: DashboardSchemas["DashboardResolvedSource"] = {
    id: "source-a",
    resource_type: "host",
    resource_id: "host-a",
    generation: "current-installation",
    revision: 7,
    type: "journald",
    capability_version: "v1",
    name: "Host Journal",
    registered_at: "2026-11-05T10:27:39+08:00",
  };
  const rows = sourceRows([
    { ...source, revision: 9, registered_at: "2026-11-05T03:30:00Z" },
    source,
    { ...source, id: "source-b", generation: "old-installation" },
    { ...source, id: "source-c", name: "Another journal rule" },
  ]);
  expect(rows).toHaveLength(3);
  expect(rows[0]?.revisions).toEqual(new Set([7, 9]));
  expect(rows[0]?.registeredAt).toBe(Date.parse(source.registered_at!));
  expect(rows.map((row) => row.source.name)).toEqual([
    "Host Journal",
    "Host Journal",
    "Another journal rule",
  ]);
});
