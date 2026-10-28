import { expect, it } from "vitest";
import { createMockApiClient } from "./index";

it("filters environment and name before paging and binds the cursor to both", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  for (const [name, environment] of [
    ["Orders one", "production"],
    ["Orders two", "production"],
    ["Orders test", "testing"],
    ["Other", "production"],
  ]) {
    const action = await api.monitoring.preview({
      kind: "application",
      operation: "create",
      application: {
        name: name!,
        environment: environment!,
        description: "",
        owner_name: "",
      },
    });
    await api.approvals.confirm(action.action_ref);
  }
  const filter = { search: " orders ", environment: "production", limit: 1 };
  const first = await api.monitoring.applications(filter);
  const second = await api.monitoring.applications({
    ...filter,
    cursor: first.next_cursor,
  });
  expect(first.next_cursor).toBeTruthy();
  expect(second.next_cursor).toBeUndefined();
  expect(new Set([...first.items, ...second.items].map((a) => a.id)).size).toBe(
    2,
  );
  expect(
    [...first.items, ...second.items].every(
      (a) => a.environment === "production",
    ),
  ).toBe(true);
  await expect(
    api.monitoring.applications({
      ...filter,
      environment: "testing",
      cursor: first.next_cursor,
    }),
  ).rejects.toMatchObject({ code: "MONITORING_CURSOR_INVALID" });
  expect(
    (await api.monitoring.applications({ ...filter, environment: "testing" }))
      .items[0]?.name,
  ).toBe("Orders test");
});
