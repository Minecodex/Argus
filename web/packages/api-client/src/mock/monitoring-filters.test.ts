import { expect, it } from "vitest";
import { createMockApiClient } from "./index";

it("filters collection instances before slicing pages and binds changed conditions to the cursor", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const collector = (
    await api.monitoring.collectors({ limit: 100 })
  ).items.find((row) => row.resource.kind === "host")!;
  const target = { kind: "host" as const, id: collector.resource.id };
  for (const [key, enabled] of [
    ["host_metrics", true],
    ["host_process_metrics", false],
    ["collector_metrics", false],
  ] as const) {
    const action = await api.monitoring.previewBuiltin({
      target,
      key,
      enabled,
      expected_version: 0,
    });
    await api.approvals.confirm(action.action_ref);
  }
  const first = await api.monitoring.collections({
    desired_enabled: false,
    limit: 1,
  });
  expect(first.items).toHaveLength(1);
  expect(first.items[0]!.desired_enabled).toBe(false);
  expect(first.next_cursor).toBeTruthy();
  const second = await api.monitoring.collections({
    desired_enabled: false,
    limit: 1,
    cursor: first.next_cursor,
  });
  expect(second.items).toHaveLength(1);
  expect(second.items[0]!.desired_enabled).toBe(false);
  expect(second.items[0]!.id).not.toBe(first.items[0]!.id);
  expect(second.next_cursor).toBeUndefined();
  await expect(
    api.monitoring.collections({
      desired_enabled: true,
      limit: 1,
      cursor: first.next_cursor,
    }),
  ).rejects.toMatchObject({ code: "MONITORING_CURSOR_INVALID" });
  const combined = await api.monitoring.collections({
    desired_enabled: false,
    integration: "host_process_metrics",
    apply_status: "pending",
    health: "unknown",
    freshness: "no_data",
  });
  expect(combined.items).toHaveLength(1);
  const none = await api.monitoring.collections({
    desired_enabled: false,
    integration: "host_process_metrics",
    health: "healthy",
  });
  expect(none.items).toHaveLength(0);
});
