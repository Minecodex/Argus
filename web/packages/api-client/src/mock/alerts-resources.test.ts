import { expect, it } from "vitest";
import { createSeedDb } from "./seed";
import { ensureAlertResources } from "./alerts-resources";
import { requireTelemetryResources } from "./telemetry-access";
import type { MockContext } from "./context";

it("registers alert fixture references in inventory and still denies revoked telemetry scope", () => {
  const db = createSeedDb(Date.parse("2026-10-07T02:32:00Z"));
  const ctx = {
    db,
    enterpriseId: () => "ent-acme",
    actor: () => ({ id: "u-root" }),
    nowIso: () => "2026-10-07T02:32:00Z",
  } as unknown as MockContext;
  ensureAlertResources(ctx, ctx.nowIso());
  const host = db.hosts.find(
    (row) => row.enterpriseId === "ent-acme" && row.name === "web-prod-01",
  )!;
  expect(host).toBeDefined();
  expect(() => requireTelemetryResources(ctx, [host.id])).not.toThrow();
  const ids = db.hosts.map((row) => row.id);
  ensureAlertResources(ctx, ctx.nowIso());
  expect(db.hosts.map((row) => row.id)).toEqual(ids);
  db.dataAuthorizationGrants!.find(
    (grant) => grant.resource_id === host.id,
  )!.active = false;
  expect(() => requireTelemetryResources(ctx, [host.id])).toThrow();
});
