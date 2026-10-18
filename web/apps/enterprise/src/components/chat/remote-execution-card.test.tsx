// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import i18n from "i18next";
import { LocaleProvider } from "@argus/ui";
import {
  ApiError,
  type RemoteExecution,
  type PendingActionPublic,
} from "@argus/api-client";
import "../../i18n";
import { RemoteExecutionCard } from "./remote-execution-card";
import { PendingActionCard } from "./pending-action-card";

const api = vi.hoisted(() => ({
  approvals: { get: vi.fn(), confirm: vi.fn(), cancel: vi.fn() },
  auth: { stepUp: vi.fn() },
  remoteExecutions: {
    get: vi.fn(),
    stop: vi.fn(),
    feedback: vi.fn(),
    inputUrl: vi.fn(),
  },
}));
vi.mock("@argus/api-client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@argus/api-client")>()),
  useApi: () => api,
}));
const action: PendingActionPublic = {
  schema_version: "argus.pending_action/v1",
  action_ref: "approved-action",
  action_type: "remote_execution.execute",
  title: "Fix service",
  summary: "Controlled change",
  risk: "write",
  preview: { id: "execution-1", conversation_id: "chat-1" },
  diff: [],
  status: "awaiting_confirmation",
  available_actions: ["confirm", "cancel"],
  expires_at: new Date(Date.now() + 600000).toISOString(),
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};
function fixture(): RemoteExecution {
  return {
    id: "execution-1",
    action_ref: action.action_ref,
    content_hash: "a".repeat(64),
    status: "awaiting_confirmation",
    stop_requested: false,
    stop_status: "",
    revision: 1,
    updated_at: new Date().toISOString(),
    spec: {
      schema_version: "argus.remote_execution/v1",
      id: "execution-1",
      conversation_id: "chat-1",
      kind: "connect",
      title: "Fix service",
      description: "Inspect, then repair",
      inputs: [],
      steps: [
        {
          id: "step-1",
          description: "Verify the service",
          target: { resource_type: "host", resource_id: "host-1" },
          identity: {
            id: "identity-1",
            username: "operator",
            sid: "S-1-5-21-123",
            version: 1,
            platform: "windows",
          },
          interpreter: "powershell",
          script: "Get-Service -Name example",
          timeout_seconds: 60,
          session_id: "session-1",
          session_generation: "generation-1",
          context_revision: 2,
        },
      ],
    },
    results: [],
  };
}
function mount(parent = false) {
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={cache}>
      <LocaleProvider>
        {parent ? (
          <PendingActionCard actionRef={action.action_ref} />
        ) : (
          <RemoteExecutionCard action={action} />
        )}
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...view, cache };
}
beforeEach(async () => {
  vi.resetAllMocks();
  await i18n.changeLanguage("zh-CN");
  api.remoteExecutions.get.mockResolvedValue(fixture());
  api.approvals.confirm.mockResolvedValue({});
  api.auth.stepUp.mockResolvedValue({});
});
afterEach(cleanup);

it("shows only consent controls before approval and replaces them with a tool record", async () => {
  const view = fixture();
  api.approvals.confirm.mockImplementation(async () => {
    api.remoteExecutions.get.mockResolvedValue({ ...view, status: "running" });
    return {};
  });
  const mounted = mount();
  await screen.findByText("Get-Service -Name example");
  expect(
    mounted.container.querySelector(".argus-command-approval"),
  ).toBeInTheDocument();
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "确认执行" }));
  await screen.findByRole("button", { name: "展开命令执行详情" });
  expect(
    mounted.container.querySelector(".argus-command-approval"),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByText("Get-Service -Name example"),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "展开命令执行详情" }));
  await screen.findByText("Get-Service -Name example");
});
it("confirms only the displayed action and does not execute on mount", async () => {
  mount();
  const button = await screen.findByRole("button", { name: "确认执行" });
  expect(api.approvals.confirm).not.toHaveBeenCalled();
  fireEvent.click(button);
  await waitFor(() =>
    expect(api.approvals.confirm).toHaveBeenCalledExactlyOnceWith(
      action.action_ref,
    ),
  );
});
it("routes a remote approval to its command card before generic diff presentation", async () => {
  api.approvals.get.mockResolvedValue({ ...action, diff: null });
  mount(true);
  await screen.findByText("Get-Service -Name example");
  expect(await screen.findByRole("button", { name: "确认执行" })).toBeEnabled();
  expect(api.approvals.confirm).not.toHaveBeenCalled();
});
it("does not stop backend execution when the card unmounts", async () => {
  const view = fixture();
  view.status = "running";
  api.remoteExecutions.get.mockResolvedValue(view);
  const mounted = mount();
  await screen.findByRole("button", { name: "展开命令执行详情" });
  expect(
    screen.queryByRole("button", { name: "停止执行" }),
  ).not.toBeInTheDocument();
  mounted.unmount();
  expect(api.remoteExecutions.stop).not.toHaveBeenCalled();
});
it("shows an unknown outcome without offering replay or a per-card stop", async () => {
  const view = fixture();
  view.status = "result_unknown";
  view.stop_status = "unknown";
  api.remoteExecutions.get.mockResolvedValue(view);
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "展开命令执行详情" }));
  await screen.findByText("停止结果未知，请根据已有证据提出恢复方案");
  expect(
    screen.queryByRole("button", { name: "停止执行" }),
  ).not.toBeInTheDocument();
  expect(api.approvals.confirm).not.toHaveBeenCalled();
});
it("never flashes manual consent for a backend automatic confirmation", async () => {
  api.remoteExecutions.get.mockResolvedValue({
    ...fixture(),
    confirmation_required: false,
  });
  const mounted = mount();
  await screen.findByRole("button", { name: "展开命令执行详情" });
  expect(
    mounted.container.querySelector(".argus-command-approval"),
  ).not.toBeInTheDocument();
  expect(api.approvals.confirm).not.toHaveBeenCalled();
});
it("retains the shared MFA confirmation flow", async () => {
  api.approvals.confirm.mockRejectedValueOnce(
    new ApiError(
      {
        code: "STEP_UP_REQUIRED",
        message_key: "errors.codes.STEP_UP_REQUIRED",
        request_id: "test",
        retryable: false,
      },
      403,
    ),
  );
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "确认执行" }));
  await screen.findByRole("dialog", { name: "验证身份后继续操作" });
  fireEvent.change(screen.getByLabelText(/验证码或恢复码/), {
    target: { value: "123456" },
  });
  fireEvent.click(screen.getByRole("button", { name: "验证并继续" }));
  await waitFor(() => expect(api.approvals.confirm).toHaveBeenCalledTimes(2));
  expect(api.approvals.confirm.mock.calls).toEqual([
    [action.action_ref],
    [action.action_ref],
  ]);
});

it("separates a completed command from incomplete output delivery", async () => {
  const view = fixture();
  view.status = "partial_failed";
  view.error_code = "REMOTE_OUTPUT_COLLECTION_FAILED";
  view.results = [
    {
      sequence: 1,
      status: "succeeded",
      exit_code: 0,
      stdout: "effect completed",
      stderr: "",
      truncated: false,
      directory: "/owned",
      session_lost: false,
      output_files: [],
      terminated: true,
      outputs_collected: false,
      observed_at: new Date().toISOString(),
    },
  ];
  api.remoteExecutions.get.mockResolvedValue(view);
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "展开命令执行详情" }));
  await screen.findByText(
    "命令已结束，输出尚未完整导入 Workspace。重传文件不会重新执行命令。",
  );
  expect(
    screen.queryByRole("button", { name: "确认执行" }),
  ).not.toBeInTheDocument();
  expect(api.approvals.confirm).not.toHaveBeenCalled();
});

it("does not offer approval after the server expires changed context", async () => {
  const view = fixture();
  view.status = "expired";
  view.error_code = "REMOTE_CONTEXT_CHANGED";
  api.remoteExecutions.get.mockResolvedValue(view);
  mount();
  await screen.findByText("确认已过期");
  expect(
    screen.queryByRole("button", { name: "确认执行" }),
  ).not.toBeInTheDocument();
});

it("shows the frozen location and reads only the related preceding batch without approval", async () => {
  const current = fixture();
  const previous = fixture();
  previous.id = previous.spec.id = "previous-1";
  previous.status = "succeeded";
  previous.spec.steps[0]!.script = "Set-Location C:\\service";
  current.status = "succeeded";
  current.spec.steps[0]!.known_directory = "C:\\service";
  current.spec.steps[0]!.interpreter_version = "PowerShell 5.1";
  current.spec.steps[0]!.previous_request_id = previous.id;
  current.spec.steps[0]!.previous_sequence = 1;
  api.remoteExecutions.get.mockImplementation(async (_conversation, id) =>
    id === previous.id ? previous : current,
  );
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "展开命令执行详情" }));
  await screen.findByText("C:\\service");
  expect(screen.getByText("PowerShell 5.1")).toBeInTheDocument();
  const disclosure = screen
    .getByText("查看前序批次（第 1 步）")
    .closest("details")!;
  disclosure.open = true;
  fireEvent(disclosure, new Event("toggle"));
  await screen.findByText("Set-Location C:\\service");
  expect(api.remoteExecutions.get).toHaveBeenCalledWith("chat-1", "previous-1");
  expect(api.approvals.confirm).not.toHaveBeenCalled();
});
