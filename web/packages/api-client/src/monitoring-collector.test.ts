import { expect, it } from "vitest";
import {
  monitoringCollectorCompatible,
  type MonitoringSchemas,
} from "./monitoring";
import {
  collectorVersion,
  collectorDistributionVersion,
} from "./generated/collector-registry";

it("does not offer the no-op DNS receiver in an otherwise supported older Collector", () => {
  const item: MonitoringSchemas["MonitoringCollectorChoice"] = {
    id: "00000000-0000-4000-8000-000000000001",
    name: "Existing collector",
    resource: { kind: "host", id: "00000000-0000-4000-8000-000000000002" },
    platform: "linux_amd64",
    status: "converged",
    version: 1,
    collector_version: "0.158.0",
    distribution_version: collectorDistributionVersion,
    components: ["dns_check", "file_storage"],
    support_status: "supported",
  };
  const integration = { component: "dns_check", platforms: ["linux_amd64"] };
  expect(monitoringCollectorCompatible(item, integration)).toBe(false);
  item.collector_version = collectorVersion;
  expect(
    monitoringCollectorCompatible(item, integration, ["file_storage"], "host"),
  ).toBe(true);
  expect(
    monitoringCollectorCompatible(item, integration, [
      "argus_application_identity",
    ]),
  ).toBe(false);
  expect(
    monitoringCollectorCompatible(item, integration, [], "kubernetes_cluster"),
  ).toBe(false);
  item.status = "uninstalling";
  expect(monitoringCollectorCompatible(item, integration)).toBe(false);
});

it("requires the verified Argus identity distribution for SDK listeners", () => {
  const item: MonitoringSchemas["MonitoringCollectorChoice"] = {
    id: "00000000-0000-4000-8000-000000000001",
    name: "Existing SDK collector",
    resource: { kind: "host", id: "00000000-0000-4000-8000-000000000002" },
    platform: "linux_amd64",
    status: "converged",
    version: 1,
    collector_version: collectorVersion,
    distribution_version: "0.3.0-plan8",
    components: [
      "otlp",
      "argus_application_auth",
      "argus_application_identity",
    ],
    support_status: "supported",
  };
  const integration = { component: "otlp", platforms: ["linux_amd64"] };
  const required = ["argus_application_auth", "argus_application_identity"];
  expect(monitoringCollectorCompatible(item, integration, required)).toBe(
    false,
  );
  item.distribution_version = collectorDistributionVersion;
  expect(monitoringCollectorCompatible(item, integration, required)).toBe(true);
  item.components = ["otlp", "argus_application_identity"];
  expect(monitoringCollectorCompatible(item, integration, required)).toBe(
    false,
  );
});
