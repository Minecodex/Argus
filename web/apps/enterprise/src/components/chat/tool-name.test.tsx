// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import i18n from "../../i18n";
import { ToolTrace } from "./tool-trace";
import { toolDisplayName } from "./tool-name";
import { chatMessagesFromEvents } from "./chat-view-model";
import {
  initialConversationProjection,
  reduceAgentEvent,
} from "./event-reducer";
import type {
  AgentEvent,
  ConversationEvent,
} from "@argus/api-client/contracts";

afterEach(async () => {
  cleanup();
  await i18n.changeLanguage("zh-CN");
});

const trace = (toolName: string, source = "argus") => ({
  callId: toolName,
  toolName,
  source,
  status: "success" as const,
  startedAt: "2026-10-13T08:00:00Z",
});

describe("localized conversation tool identity", () => {
  it("changes the rendered and accessible name with locale while preserving details", async () => {
    await i18n.changeLanguage("zh-CN");
    render(
      <ToolTrace
        toolCalls={[{ ...trace("host.get"), input: { host_id: "host-1" } }]}
      />,
    );
    const toggle = screen.getByRole("button", { name: /主机获取/ });
    expect(toggle).toHaveTextContent("主机获取");
    expect(screen.queryByText("host.get")).not.toBeInTheDocument();
    fireEvent.click(toggle);
    expect(screen.getByText("host.get", { selector: "code" })).toBeVisible();
    await act(() => i18n.changeLanguage("en-US"));
    expect(screen.getByRole("button", { name: /Get host/ })).toHaveTextContent(
      "Get host",
    );
    expect(screen.getByText(/Tool identifier:/)).toHaveTextContent("host.get");
    fireEvent.click(
      screen.getByText("Input parameters", { selector: "summary" }),
    );
    expect(screen.getByText(/host-1/, { selector: "pre" })).toBeVisible();
  });

  it("covers the requested business names and keeps confirmation previews distinct", async () => {
    await i18n.changeLanguage("zh-CN");
    const tools = [
      ["tool.search", "工具搜索"],
      ["tool.describe", "工具详情"],
      ["connect", "远程命令"],
      ["workflow.remote_plan.preview", "远程修复计划"],
      ["telemetry.dashboard.get", "仪表盘获取"],
      ["telemetry.promql.query", "指标查询"],
      ["host.list", "主机列表"],
      ["alert.event.query", "告警列表"],
      ["alert.policy.enable.preview", "启用告警策略预览"],
    ];
    render(
      <>
        {tools.map(([id]) => (
          <ToolTrace key={id} toolCalls={[trace(id!)]} />
        ))}
      </>,
    );
    for (const [id, label] of tools) {
      expect(
        screen.getByRole("button", { name: new RegExp(label!) }),
      ).toHaveTextContent(label!);
      expect(screen.queryByText(id!)).not.toBeInTheDocument();
    }
    expect(
      toolDisplayName(i18n.t.bind(i18n), "telemetry.dashboard.query"),
    ).toBe("仪表盘查询");
    expect(
      toolDisplayName(i18n.t.bind(i18n), "telemetry.dashboard.query.get"),
    ).toBe("仪表盘查询结果");
  });

  it("keeps unknown and external names intact even when an external name matches a native ID", async () => {
    await i18n.changeLanguage("en-US");
    render(<ToolTrace toolCalls={[trace("host.get", "customer_mcp")]} />);
    expect(screen.getByRole("button", { name: /host\.get/ })).toHaveTextContent(
      "host.get",
    );
    expect(toolDisplayName(i18n.t.bind(i18n), "external.inspect")).toBe(
      "external.inspect",
    );
  });

  it("preserves the tool provider through history and streaming projections", () => {
    const base = {
      event_id: "result",
      sequence: 1,
      conversation_id: "conversation",
      run_id: "run",
      occurred_at: "2026-10-13T08:00:00Z",
      payload: {
        tool_call_id: "call",
        tool_id: "tool.invoke",
        target_tool_id: "host.get",
        source: "customer_mcp",
        status: "succeeded",
      },
    };
    const history = chatMessagesFromEvents([
      {
        ...base,
        schema_version: "argus.conversation_event/v1",
        event_type: "tool_call_result",
      } as ConversationEvent,
    ]);
    expect(history[0]?.toolCalls?.[0]).toMatchObject({
      toolName: "host.get",
      source: "customer_mcp",
    });
    const current = reduceAgentEvent(initialConversationProjection, {
      ...base,
      schema_version: "argus.agent_event/v1",
      event_type: "tool_call_completed",
    } as AgentEvent);
    expect(current.tool_calls.get("call")).toMatchObject({
      toolName: "host.get",
      source: "customer_mcp",
    });
  });
});
