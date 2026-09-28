import { expect, it } from "vitest";
import { createMockApiClient } from "./index";
import { emptyDashboardSpec } from "../dashboard";

it("stores explicit dashboard choices, inherits follow-ups, rejects stale tabs and supports clearing", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const conversation = await api.conversations.create();
  const draft = await api.dashboards.createDraft({
    name: "Chat overview",
    description: "",
    spec: emptyDashboardSpec(),
    proposed_bindings: [],
  });
  const preview = await api.dashboards.preview(draft.id, draft.draft_version);
  await api.approvals.confirm(preview.action_ref);
  const id = (await api.dashboards.list())[0]!.id;
  const selection = {
    mode: "analyze" as const,
    dashboard_ids: [id],
    expected_version: 0,
  };
  await api.conversations.preflight(conversation.id, {
    content: "检查",
    dashboard_context: selection,
  });
  expect((await api.conversations.dashboardContext(conversation.id)).mode).toBe(
    "none",
  );
  for await (const _ of api.conversations.sendMessage(conversation.id, {
    content: "检查",
    dashboard_context: selection,
  })) {
    void _;
  }
  expect(
    await api.conversations.dashboardContext(conversation.id),
  ).toMatchObject({ mode: "analyze", dashboard_ids: [id], version: 1 });
  for await (const _ of api.conversations.sendMessage(conversation.id, {
    content: "继续",
  })) {
    void _;
  }
  expect(
    (await api.conversations.dashboardContext(conversation.id)).dashboard_ids,
  ).toEqual([id]);
  await expect(
    api.conversations.preflight(conversation.id, {
      content: "旧窗口",
      dashboard_context: selection,
    }),
  ).rejects.toMatchObject({ code: "TOOL_CONFIGURATION_CHANGED" });
  for await (const _ of api.conversations.sendMessage(conversation.id, {
    content: "离开",
    dashboard_context: { mode: "none", dashboard_ids: [], expected_version: 1 },
  })) {
    void _;
  }
  expect((await api.conversations.dashboardContext(conversation.id)).mode).toBe(
    "none",
  );
  for await (const _ of api.conversations.sendMessage(conversation.id, {
    content: "/create-dashboard New one",
  })) {
    void _;
  }
  expect((await api.conversations.dashboardContext(conversation.id)).mode).toBe(
    "create",
  );
  const events = await api.conversations.listEvents(conversation.id);
  expect(JSON.stringify(events)).toContain("dashboard_context");
});
