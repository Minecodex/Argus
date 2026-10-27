import { expect, it } from "vitest";
import { createMockApiClient } from "./index";

it("keeps fixed own metrics separate from remote targets and other own instances", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const collector = (
    await api.monitoring.collectors({ limit: 100 })
  ).items.find(
    (row) => row.resource.kind === "host",
  )!;
  const target = { kind: "host" as const, id: collector.resource.id };
  const before = await api.monitoring.builtins(target);
  expect(before).toHaveLength(5);
  expect(before.find((row) => row.definition.key === "host_windows_service_metrics")?.available).toBe(false);
  for (const key of ["host_metrics", "host_process_metrics", "host_systemd_metrics", "collector_metrics"] as const) {
    const action = await api.monitoring.previewBuiltin({
      target,
      key,
      enabled: true,
      expected_version: 0,
    });
    await api.approvals.confirm(action.action_ref);
  }
  const enabled = await api.monitoring.builtins(target),
    host = enabled.find((row) => row.definition.key === "host_metrics")!;
  const action = await api.monitoring.previewBuiltin({
    target,
    key: "host_metrics",
    enabled: false,
    expected_version: host.collection!.version,
  });
  await api.approvals.confirm(action.action_ref);
  const after = await api.monitoring.builtins(target);
  expect(
    after.find((row) => row.definition.key === "host_metrics")?.collection
      ?.desired_enabled,
  ).toBe(false);
  expect(
    after.find((row) => row.definition.key === "collector_metrics")?.collection
      ?.desired_enabled,
  ).toBe(true);
  expect((await api.monitoring.targets()).items).toHaveLength(0);
  for (const key of ["host_process_metrics", "host_systemd_metrics"] as const) {
    expect(after.find((row) => row.definition.key === key)?.collection?.desired_enabled).toBe(true);
  }
  expect(after.find((row) => row.definition.key === "host_metrics")?.id).toBe(
    host.id,
  );
});
