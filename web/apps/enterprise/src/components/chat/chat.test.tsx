// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatMessage, ToolCallTrace } from "./chat-view-model";
import {
  ApiProvider,
  createMockApiClient,
  type AlertPolicySpec,
  type MockApiClient,
} from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import "../../i18n";
import { ChatMessageItem } from "./message-item";
import { ChatMessageList } from "./message-list";
import { PendingActionCard } from "./pending-action-card";
import { ToolTrace } from "./tool-trace";

// @argus/ui 的 LocaleProvider 依据 localStorage/navigator 判定语言，固定为中文。
window.localStorage.setItem("argus.locale", "zh-CN");

// vite.config 未开启 vitest globals，RTL 的自动 cleanup 不生效，手动清理。
afterEach(cleanup);

function createWrapper(client: MockApiClient) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <LocaleProvider>
        <ApiProvider client={client}>
          <QueryClientProvider client={queryClient}>
            {children}
          </QueryClientProvider>
        </ApiProvider>
      </LocaleProvider>
    );
  };
}

function login(client: MockApiClient) {
  return client.auth.login({ username: "root", password: "123456" });
}

const NOW = new Date().toISOString();

const TOOL_CALLS: ToolCallTrace[] = [
  {
    callId: "call-1",
    toolName: "host.resolve_context",
    status: "success",
    startedAt: NOW,
    durationMs: 320,
    summary: "调用成功",
  },
  {
    callId: "call-2",
    toolName: "host.test_connection",
    status: "running",
    startedAt: NOW,
  },
];

function makeMessage(patch: Partial<ChatMessage>): ChatMessage {
  return {
    id: "msg-t1",
    conversationId: "conv-1",
    role: "assistant",
    content: "",
    createdAt: NOW,
    ...patch,
  };
}

describe("ToolTrace", () => {
  it("默认折叠，点击展开后展示工具名称与耗时", () => {
    render(<ToolTrace toolCalls={TOOL_CALLS} />);
    expect(screen.queryByText("host.resolve_context")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "展开工具调用" }));
    expect(screen.getByText("host.resolve_context")).toBeInTheDocument();
    expect(screen.getByText("host.test_connection")).toBeInTheDocument();
    expect(screen.getByText("320ms")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "收起工具调用" }));
    expect(screen.queryByText("host.resolve_context")).not.toBeInTheDocument();
  });
});

describe("ChatMessageItem", () => {
  it("渲染用户消息与 AI 消息（含折叠的工具 trace）", () => {
    const client = createMockApiClient({ persist: false, delay: 0 });
    const wrapper = createWrapper(client);
    render(
      <ChatMessageItem
        message={makeMessage({
          id: "msg-u1",
          role: "user",
          content: "帮我新增一台 Web 主机",
        })}
      />,
      { wrapper },
    );
    expect(screen.getByTestId("chat-message-user")).toHaveTextContent(
      "帮我新增一台 Web 主机",
    );

    render(
      <ChatMessageItem
        message={makeMessage({
          id: "msg-a1",
          content: "已完成目标解析与连接测试。",
          toolCalls: TOOL_CALLS,
        })}
      />,
      { wrapper },
    );
    expect(screen.getByTestId("chat-message-assistant")).toHaveTextContent(
      "已完成目标解析与连接测试。",
    );
    expect(screen.getByTestId("tool-trace")).toBeInTheDocument();
  });
});

describe("ChatMessageList", () => {
  it("shows one assistant identity across consecutive prose and tool steps, then starts a reply after user input", () => {
    const wrapper = createWrapper(
      createMockApiClient({ persist: false, delay: 0 }),
    );
    const { container, rerender } = render(
      <ChatMessageList
        messages={[
          makeMessage({ id: "user-1", role: "user", content: "检查主机" }),
          makeMessage({ id: "planning", content: "先搜索工具。" }),
          makeMessage({
            id: "tool",
            kind: "tool",
            toolCalls: [TOOL_CALLS[0]!],
          }),
          makeMessage({
            id: "continuation",
            content: "继续核对状态。",
            runId: "another-run",
          }),
        ]}
        pendingUser={null}
        streaming={makeMessage({
          id: "live",
          content: "当前指标正常。",
          isStreaming: true,
        })}
      />,
      { wrapper },
    );
    expect(screen.getAllByTestId("chat-assistant-heading")).toHaveLength(1);
    expect(screen.getAllByTestId("chat-assistant-reply")).toHaveLength(1);
    const body = screen.getByTestId("chat-assistant-reply");
    expect(body.textContent).toMatch(
      /先搜索工具。[\s\S]*host.resolve_context[\s\S]*继续核对状态。[\s\S]*当前指标正常。/,
    );
    expect(
      container.querySelectorAll(".argus-chat-message__avatar"),
    ).toHaveLength(1);
    rerender(
      <ChatMessageList
        messages={[
          makeMessage({ id: "user-1", role: "user", content: "检查主机" }),
          makeMessage({ id: "planning", content: "先搜索工具。" }),
          makeMessage({ id: "user-2", role: "user", content: "继续查告警" }),
          makeMessage({ id: "answer", content: "正在查询告警。" }),
        ]}
        pendingUser={null}
        streaming={null}
      />,
    );
    expect(screen.getAllByTestId("chat-assistant-heading")).toHaveLength(2);
  });

  it("places optimistic user input before streamed output and begins a new reply", () => {
    const wrapper = createWrapper(
      createMockApiClient({ persist: false, delay: 0 }),
    );
    const { container } = render(
      <ChatMessageList
        messages={[makeMessage({ id: "old", content: "上次结果" })]}
        pendingUser={makeMessage({
          id: "local-user",
          role: "user",
          content: "新的问题",
          createdAt: new Date(Date.parse(NOW) + 1000).toISOString(),
        })}
        streaming={makeMessage({
          id: "new",
          content: "新的回答",
          createdAt: new Date(Date.parse(NOW) + 2000).toISOString(),
        })}
      />,
      { wrapper },
    );
    expect(
      Array.from(
        container.querySelector(".argus-chat-stream__inner")!.children,
      ).map((element) => element.getAttribute("data-testid")),
    ).toEqual([
      "chat-assistant-reply",
      "chat-message-user",
      "chat-assistant-reply",
    ]);
  });
  it("查询数据落地时不重复显示乐观消息和流式消息", () => {
    const userMessage = makeMessage({
      id: "msg-user-persisted",
      role: "user",
      content: "主机容量表",
    });
    const assistantMessage = makeMessage({
      id: "msg-assistant-persisted",
      content: "已汇总主机容量",
    });

    render(
      <ChatMessageList
        messages={[userMessage, assistantMessage]}
        pendingUser={{ ...userMessage, id: "local-user-1" }}
        streaming={assistantMessage}
      />,
    );

    expect(screen.getAllByText("主机容量表")).toHaveLength(1);
    expect(screen.getAllByText("已汇总主机容量")).toHaveLength(1);
  });
});

describe("PendingActionCard", () => {
  it("告警确认前展示已授权渠道名称，业务配置保持不变", async () => {
    const client = createMockApiClient({ persist: false, delay: 0 });
    await login(client);
    const before = structuredClone(
      await client.alerts.configurations("policy"),
    );
    const action = await client.alerts.preview({
      kind: "policy",
      operation: "create",
      expected_version: 0,
      name: "待确认双指标策略",
      spec: structuredClone(before[0]!.spec) as AlertPolicySpec,
    });

    render(<PendingActionCard actionRef={action.action_ref} />, {
      wrapper: createWrapper(client),
    });

    await screen.findByText("生产事件群 · 基础设施邮件");
    expect(screen.queryByText("c3 · c4")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "确认提交" })).toBeEnabled();
    expect(await client.alerts.configurations("policy")).toEqual(before);
  });

  it("[确认执行] 直接调 approvals.confirm，状态原地流转到成功", async () => {
    const client = createMockApiClient({
      persist: false,
      delay: 0,
      stepDelay: 10,
    });
    await login(client);
    const action = await client.approvals.preview({
      tool: "host.create",
      title: "新增主机",
      conversation_id: "conv-1",
      input_data: { name: "host-test-x", address: "10.0.0.99" },
    });
    const confirmSpy = vi.spyOn(client.approvals, "confirm");

    render(<PendingActionCard actionRef={action.action_ref} />, {
      wrapper: createWrapper(client),
    });

    // 预览加载完成：结构化动作按当前语言展示，且保留资源名称与地址。
    await screen.findByText("新增主机 host-test-x");
    expect(screen.getByText("创建主机 host-test-x")).toBeInTheDocument();
    expect(screen.getByText("10.0.0.99")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "确认执行" }));
    await waitFor(() =>
      expect(confirmSpy).toHaveBeenCalledWith(action.action_ref),
    );
    // mock 任务步骤按 stepDelay 推进，轮询后进入成功终态
    await screen.findByText(/主机 host-test-x 已创建/, undefined, {
      timeout: 4_000,
    });
    expect(screen.queryByText("查看任务")).not.toBeInTheDocument();
    expect(screen.getByText("执行成功", { exact: true })).toBeInTheDocument();
  });

  it("[取消] 调 approvals.cancel，状态流转为已取消", async () => {
    const client = createMockApiClient({ persist: false, delay: 0 });
    await login(client);
    const action = await client.approvals.preview({
      tool: "host.create",
      title: "新增主机",
      conversation_id: "conv-1",
      input_data: { name: "host-test-y", address: "10.0.0.98" },
    });
    const cancelSpy = vi.spyOn(client.approvals, "cancel");

    render(<PendingActionCard actionRef={action.action_ref} />, {
      wrapper: createWrapper(client),
    });

    await screen.findByText("新增主机 host-test-y");
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    await waitFor(() =>
      expect(cancelSpy).toHaveBeenCalledWith(action.action_ref),
    );
    await screen.findByText("操作已取消");
  });
});
