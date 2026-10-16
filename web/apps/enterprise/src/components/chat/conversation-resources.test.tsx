// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, createMockApiClient } from "@argus/api-client";
import { afterEach, expect, it, vi } from "vitest";
import type { ChatMessage } from "./chat-view-model";
import { useConversationResources } from "./conversation-resources";
let allowed = new Set(["host.read", "telemetry.dashboard.read"]);
vi.mock("../../lib/permissions", () => ({ useMyPermissions: () => allowed }));
afterEach(() => {
  cleanup();
  allowed = new Set(["host.read", "telemetry.dashboard.read"]);
});
function View({ messages }: { messages: ChatMessage[] }) {
  const refs = useConversationResources(messages);
  return (
    <div>
      {refs.map((ref) => (
        <a key={`${ref.type}:${ref.id}`} href={ref.href}>
          {ref.unavailable ? "访问不可用" : (ref.name ?? "读取中")}
        </a>
      ))}
    </div>
  );
}
it("resolves exact business resource names and destinations while excluding ToolCall IDs", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  const get = vi
    .spyOn(api.presentations, "get")
    .mockResolvedValue({
      tool_call_id: "toolcall-uuid",
      runtime: "argus-template/v1",
      template_hash: "hash",
      template_source: "",
      detail_data: {},
      resource_refs: [{ id: "host-id", type: "host" }],
      result_refs: [],
      status: "ready",
    });
  vi.spyOn(api.hosts, "get").mockResolvedValue({
    name: "argus-test-host",
  } as Awaited<ReturnType<typeof api.hosts.get>>);
  vi.spyOn(api.dashboards, "get").mockResolvedValue({
    dashboard: { name: "Linux 主机总览" },
  } as Awaited<ReturnType<typeof api.dashboards.get>>);
  const queryClient = new QueryClient();
  const messages: ChatMessage[] = [
    {
      id: "event",
      conversationId: "conversation",
      role: "assistant",
      content: "",
      createdAt: "2026-10-12T13:40:00Z",
      presentations: ["toolcall-uuid"],
      dashboardContext: { mode: "analyze", dashboard_ids: ["dashboard-id"] },
    },
  ];
  render(
    <ApiProvider client={api}>
      <QueryClientProvider client={queryClient}>
        <View messages={messages} />
      </QueryClientProvider>
    </ApiProvider>,
  );
  await waitFor(() =>
    expect(
      screen.getByRole("link", { name: "argus-test-host" }),
    ).toHaveAttribute("href", "/hosts/host-id"),
  );
  expect(screen.getByRole("link", { name: "Linux 主机总览" })).toHaveAttribute(
    "href",
    "/dashboards/dashboard-id",
  );
  expect(screen.queryByText("toolcall-uuid")).toBeNull();
  expect(get).toHaveBeenCalledWith("conversation", "toolcall-uuid");
});
it("hides a previously cached name when read permission is revoked", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  vi.spyOn(api.presentations, "get").mockResolvedValue({
    tool_call_id: "call",
    runtime: "argus-template/v1",
    template_hash: "hash",
    template_source: "",
    detail_data: {},
    resource_refs: [{ id: "host", type: "host" }],
    result_refs: [],
    status: "ready",
  });
  const get = vi
    .spyOn(api.hosts, "get")
    .mockResolvedValue({ name: "private host" } as Awaited<
      ReturnType<typeof api.hosts.get>
    >);
  const queryClient = new QueryClient();
  const messages: ChatMessage[] = [
    {
      id: "event",
      conversationId: "conversation",
      role: "assistant",
      content: "",
      createdAt: "2026-10-12T13:40:00Z",
      presentations: ["call"],
    },
  ];
  const tree = () => (
    <ApiProvider client={api}>
      <QueryClientProvider client={queryClient}>
        <View messages={messages} />
      </QueryClientProvider>
    </ApiProvider>
  );
  const view = render(tree());
  await screen.findByText("private host");
  allowed = new Set();
  view.rerender(tree());
  expect(screen.queryByText("private host")).toBeNull();
  expect(screen.getByText("访问不可用")).not.toHaveAttribute("href");
  expect(get).toHaveBeenCalledTimes(1);
});
