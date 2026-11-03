// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
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
  const get = vi.spyOn(api.presentations, "get").mockResolvedValue({
    tool_call_id: "toolcall-uuid",
    runtime: "argus-template/v1",
    template_hash: "hash",
    template_source: "",
    detail_data: {},
    resource_refs: [{ id: "host-id", type: "host" }],
    result_refs: [],
    status: "ready",
  });
  vi.spyOn(api.telemetry, "resources").mockResolvedValue({
    items: [
      {
        id: "host-id",
        type: "host",
        name: "argus-test-host",
        environment: "production",
      },
    ],
  });
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
  const get = vi.spyOn(api.telemetry, "resources").mockResolvedValue({
    items: [
      {
        id: "host",
        type: "host",
        name: "private host",
        environment: "production",
      },
    ],
  });
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

it("hides all seven cached resource names and links while data grants are rechecked", async () => {
  allowed = new Set(["*"]);
  const api = createMockApiClient({ persist: false, delay: 0 });
  const refs = (
    [
      "host",
      "kubernetes_cluster",
      "kubernetes_namespace",
      "application",
      "monitoring_target",
      "dashboard",
      "connector",
    ] as const
  ).map((type) => ({ type, id: `${type}-id` }));
  vi.spyOn(api.presentations, "get").mockResolvedValue({
    tool_call_id: "call",
    runtime: "argus-template/v2",
    template_hash: "hash",
    template_source: "",
    detail_data: {},
    resource_refs: refs,
    result_refs: [],
    status: "ready",
  });
  let checking = false,
    finish!: () => void;
  const pending = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const result = async (value: unknown) => {
    if (checking) {
      await pending;
      throw new Error("Resource unavailable");
    }
    return value as never;
  };
  const name = (type: string) => `private-${type}`;
  vi.spyOn(api.hosts, "get").mockImplementation(async () =>
    result({ name: name("host") }),
  );
  vi.spyOn(api.hosts, "list").mockResolvedValue({
    items: [{ id: "host-id", connector_id: "connector-id" }],
  } as never);
  vi.spyOn(api.kubernetes, "getCluster").mockImplementation(async () =>
    result({ name: name("kubernetes_cluster") }),
  );
  vi.spyOn(api.monitoring, "application").mockImplementation(async () =>
    result({ name: name("application") }),
  );
  vi.spyOn(api.dashboards, "get").mockImplementation(async () =>
    result({ dashboard: { name: name("dashboard") } }),
  );
  vi.spyOn(api.connectors, "get").mockImplementation(async () =>
    result({ name: name("connector") }),
  );
  vi.spyOn(api.telemetry, "resources").mockImplementation(async ({ kind }) =>
    result({ items: [{ id: `${kind}-id`, type: kind, name: name(kind) }] }),
  );
  const cache = new QueryClient();
  const messages: ChatMessage[] = [
    {
      id: "event",
      conversationId: "seven-types",
      role: "assistant",
      content: "",
      createdAt: "2026-10-12T13:40:00Z",
      presentations: ["call"],
    },
  ];
  render(
    <ApiProvider client={api}>
      <QueryClientProvider client={cache}>
        <View messages={messages} />
      </QueryClientProvider>
    </ApiProvider>,
  );
  await waitFor(() => {
    for (const ref of refs)
      expect(
        screen.getByRole("link", { name: name(ref.type) }),
      ).toHaveAttribute("href");
  });
  checking = true;
  await act(async () => {
    void cache.invalidateQueries({ queryKey: ["conversation-resource-name"] });
  });
  await waitFor(() =>
    expect(cache.isFetching({ queryKey: ["conversation-resource-name"] })).toBe(
      7,
    ),
  );
  for (const ref of refs)
    expect(screen.queryByText(name(ref.type))).not.toBeInTheDocument();
  expect(document.querySelectorAll("a[href]")).toHaveLength(0);
  await act(async () => {
    finish();
  });
  await waitFor(() =>
    expect(screen.getAllByText("访问不可用")).toHaveLength(7),
  );
  expect(document.querySelectorAll("a[href]")).toHaveLength(0);
});
