import { expect, it } from "vitest";
import { createDashboardDomains } from "./dashboard";
import { createSeedDb } from "./seed";
import type { MockContext } from "./context";
import { emptyDashboardSpec } from "../dashboard";
import type { MonitoringSchemas } from "../monitoring";

it("rechecks read-only default discovery after a resource grant or collection disappears", async () => {
  const db = createSeedDb();
  const member = db.enterpriseUsers.find((row) => row.userId === "u-root")!;
  const enterprise = member.enterpriseId;
  for (const role of db.roles) role.permissions = ["telemetry.dashboard.read"];
  const target: MonitoringSchemas["MonitoringTarget"] = {
    id: crypto.randomUUID(),
    engine: "mysql",
    kind: "database",
    identity: { endpoint: "db.test:3306" },
    name: "Authorized MySQL",
    status: "active",
    version: 1,
  };
  (db.monitoring ??= {})[enterprise] = {
    applications: [],
    services: [],
    bindings: [],
    targets: [target],
  };
  const grant = {
    subject_type: "user" as const,
    subject_id: member.userId,
    resource_type: "monitoring_target" as const,
    resource_id: target.id,
    active: true,
  };
  db.dataAuthorizationGrants = [grant];
  const ctx = {
    db,
    actor: () => ({ id: member.userId, displayName: "Reader" }),
    enterpriseId: () => enterprise,
    nowIso: () => new Date().toISOString(),
    pause: async () => {},
    save: () => {},
  } as MockContext;
  const { dashboards } = createDashboardDomains(ctx);
  const items = await dashboards.list();
  const managed = items.find(
    (row) => row.managed_template_key === "mysql.overview",
  )!;
  expect(managed).toBeTruthy();
  expect(
    items.some((row) => row.managed_template_key === "argus.model-governance"),
  ).toBe(false);
  expect((await dashboards.get(managed.id)).revision.spec.panels).toHaveLength(
    6,
  );
  await expect(
    dashboards.createDraft({
      name: "Denied",
      description: "",
      spec: emptyDashboardSpec(),
      proposed_bindings: [],
    }),
  ).rejects.toMatchObject({ code: "DASHBOARD_DENIED" });

  grant.active = false;
  expect((await dashboards.list()).some((row) => row.id === managed.id)).toBe(
    false,
  );
  // Managed definitions can be read by known ID; resource query authorization
  // is separate. Revocation must change discovery without deleting definitions.
  expect((await dashboards.get(managed.id)).dashboard.id).toBe(managed.id);
  grant.active = true;
  expect((await dashboards.list()).some((row) => row.id === managed.id)).toBe(
    true,
  );
  target.status = "archived";
  expect((await dashboards.list()).some((row) => row.id === managed.id)).toBe(
    false,
  );
});
