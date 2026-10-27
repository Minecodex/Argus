import { expect, it } from "vitest";
import { createMockApiClient } from "./index";
import { emptyDashboardSpec } from "../dashboard";

it("pages and searches many applications without widening explicit empty query scope", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  for (let i = 0; i < 65; i++) {
    const action = await api.monitoring.preview({
      kind: "application",
      operation: "create",
      application: {
        name: `Orders ${String(i).padStart(3, "0")}`,
        environment: i % 2 ? "testing" : "production",
        description: "",
        owner_name: "",
      },
    });
    await api.approvals.confirm(action.action_ref);
  }
  const first = await api.dashboards.scopeResources({
    kind: "application",
    limit: 30,
  });
  const second = await api.dashboards.scopeResources({
    kind: "application",
    limit: 30,
    cursor: first.next_cursor,
  });
  const third = await api.dashboards.scopeResources({
    kind: "application",
    limit: 30,
    cursor: second.next_cursor,
  });
  expect([first.items.length, second.items.length, third.items.length]).toEqual(
    [30, 30, 5],
  );
  expect(
    new Set(
      [...first.items, ...second.items, ...third.items].map((row) => row.id),
    ).size,
  ).toBe(65);
  await expect(
    api.dashboards.scopeResources({ kind: "host", cursor: first.next_cursor }),
  ).rejects.toBeTruthy();
  const found = await api.dashboards.scopeResources({
    kind: "application",
    search: "Orders 064",
  });
  expect(found.items).toHaveLength(1);
  const spec = emptyDashboardSpec();
  const draft = await api.dashboards.createDraft({
    name: "Explicit scope",
    description: "",
    spec,
    proposed_bindings: [],
  });
  await api.approvals.confirm(
    (await api.dashboards.preview(draft.id, 1)).action_ref,
  );
  const dashboard = (await api.dashboards.list())[0]!;
  const result = await api.dashboards.execute(dashboard.id, {
    resource_mode: "explicit",
    resource_ids: [],
  });
  expect(result.resources).toEqual([]);
  expect(result.scope_empty).toBe(true);
});
