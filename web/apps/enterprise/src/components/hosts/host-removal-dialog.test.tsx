// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "i18next";
import { ApiError } from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";

import "../../i18n";
import { HostRemovalDialog } from "./host-removal-dialog";
import type { RemovalTarget } from "./host-removal-target";

const api = vi.hoisted(() => ({
  hosts: {
    getRemovalConnectionDefaults: vi.fn(),
    createConnectionTest: vi.fn(),
    getConnectionTest: vi.fn(),
    previewRemoval: vi.fn(),
    previewDeleteResource: vi.fn(),
    getRemovalOperation: vi.fn(),
  },
  connectors: { previewDeleteBastionScope: vi.fn() },
  secrets: { listCredentials: vi.fn() },
  approvals: { cancel: vi.fn() },
}));

vi.mock("@argus/api-client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@argus/api-client")>()),
  useApi: () => api,
}));
vi.mock("./pending-action-confirm", () => ({
  PendingActionConfirm: ({
    confirmLabel,
    onCancel,
    onDone,
  }: {
    confirmLabel?: string;
    onCancel?: () => void;
    onDone?: (result: object) => void;
  }) => (
    <div>
      <p>Review removal action</p>
      <button onClick={() => onDone?.({})}>{confirmLabel}</button>
      <button onClick={onCancel}>Cancel deletion preview</button>
    </div>
  ),
}));

const target = {
  type: "managed_host",
  id: "host-123",
  hostId: "host-123",
  name: "123",
  expectedVersion: 3,
  platform: "linux",
  address: "192.168.0.101",
  port: 2222,
  installMethod: "ssh",
  sshPath: "direct_executor",
  status: "active",
  connectionStatus: "offline",
  registeredConnectorId: "connector-123",
  onboardingState: "registered",
} as RemovalTarget;
const queries: QueryClient[] = [];

beforeEach(async () => {
  vi.resetAllMocks();
  window.localStorage.setItem("argus.locale", "zh-CN");
  await i18n.changeLanguage("zh-CN");
  api.hosts.getRemovalConnectionDefaults.mockResolvedValue({
    status: "available",
    username: "install-admin",
    credential_id: "credential-1",
    credential_name: "SSH credential",
  });
  api.secrets.listCredentials.mockResolvedValue([]);
  api.hosts.createConnectionTest.mockResolvedValue({
    id: "test-1",
    status: "queued",
  });
  api.hosts.getConnectionTest.mockResolvedValue({
    id: "test-1",
    status: "succeeded",
  });
  api.hosts.previewRemoval.mockResolvedValue({
    action_ref: "action-1",
    preview: {},
  });
  api.hosts.previewDeleteResource.mockResolvedValue({
    action_ref: "delete-host-action",
    preview: {},
  });
  api.connectors.previewDeleteBastionScope.mockResolvedValue({
    action_ref: "delete-bastion-action",
    preview: {},
  });
  api.hosts.getRemovalOperation.mockResolvedValue({
    id: "operation-1",
    status: "running",
    stage: "draining",
    delivery_method: "ssh",
    events: [],
  });
  api.approvals.cancel.mockResolvedValue(undefined);
});
afterEach(() => {
  cleanup();
  queries.forEach((query) => query.clear());
  queries.length = 0;
  vi.useRealTimers();
});

function show(overrides: Partial<RemovalTarget> = {}) {
  const query = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  queries.push(query);
  const onChanged = vi.fn();
  const onOpenChange = vi.fn();
  const renderTarget = (next: RemovalTarget | null) => (
    <QueryClientProvider client={query}>
      <LocaleProvider>
        <HostRemovalDialog
          target={next}
          onChanged={onChanged}
          onOpenChange={onOpenChange}
        />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const result = render(renderTarget({ ...target, ...overrides }));
  return {
    ...result,
    onChanged,
    onOpenChange,
    rerenderTarget: (next: RemovalTarget | null) =>
      result.rerender(renderTarget(next)),
  };
}

async function startSSH() {
  await screen.findByText(/账号：install-admin/);
  fireEvent.click(screen.getByRole("button", { name: "生成卸载预览" }));
  await waitFor(() =>
    expect(api.hosts.createConnectionTest).toHaveBeenCalledTimes(1),
  );
  expect(api.hosts.createConnectionTest.mock.calls[0]?.[0]).not.toHaveProperty(
    "onboarding_control_path",
  );
}

describe("removal choices", () => {
  it("previews deletion without SSH for a Host that never registered", async () => {
    const view = show({
      registeredConnectorId: undefined,
      onboardingState: "install_failed",
    } as never);
    expect(
      screen.getByRole("heading", { name: "删除未完成的安装记录" }),
    ).toBeVisible();
    expect(api.hosts.getRemovalConnectionDefaults).not.toHaveBeenCalled();
    expect(api.hosts.createConnectionTest).not.toHaveBeenCalled();
    expect(api.hosts.previewRemoval).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "删除记录" }));
    expect(await screen.findByText("Review removal action")).toBeVisible();
    expect(api.hosts.previewDeleteResource).toHaveBeenCalledWith("host-123", 3);
    expect(screen.getByRole("button", { name: "确认删除" })).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "确认删除" }));
    expect(view.onChanged).toHaveBeenCalled();
    expect(view.onOpenChange).toHaveBeenCalledWith(false);
  });

  it("labels an in-flight Host installation as cancellation and deletion", () => {
    show({
      registeredConnectorId: undefined,
      onboardingState: "installing",
    } as never);
    expect(
      screen.getByRole("button", { name: "取消安装并删除" }),
    ).toBeVisible();
  });

  it("keeps a failed projection on the uninstall path when a Connector is registered", async () => {
    show({ onboardingState: "install_failed" });
    expect(await screen.findByText(/账号：install-admin/)).toBeVisible();
    expect(screen.getByText("SSH 自动卸载", { exact: true })).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "删除记录" }),
    ).not.toBeInTheDocument();
  });

  it("uses the Bastion Scope delete preview for an unregistered Bastion", async () => {
    show({
      type: "bastion_scope",
      id: "scope-123",
      registeredConnectorId: undefined,
      onboardingState: "install_failed",
    } as never);
    fireEvent.click(screen.getByRole("button", { name: "删除记录" }));
    await screen.findByText("Review removal action");
    expect(api.connectors.previewDeleteBastionScope).toHaveBeenCalledWith(
      "scope-123",
      3,
    );
    expect(api.hosts.previewDeleteResource).not.toHaveBeenCalled();
  });

  it("treats a not-installed defaults response as authoritative", async () => {
    api.hosts.getRemovalConnectionDefaults.mockRejectedValue(
      new ApiError(
        {
          code: "HOST_REMOVAL_NOT_INSTALLED",
          message_key: "errors.host_removal.not_installed",
          request_id: "request-not-installed",
          retryable: false,
        },
        409,
      ),
    );
    show();
    expect(
      await screen.findByRole("heading", {
        name: "删除未完成的安装记录",
      }),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "删除记录" })).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "重试加载" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByLabelText("SSH 账号")).not.toBeInTheDocument();
    expect(api.secrets.listCredentials).not.toHaveBeenCalled();
    expect(api.hosts.createConnectionTest).not.toHaveBeenCalled();
    expect(api.hosts.previewRemoval).not.toHaveBeenCalled();
  });

  it("opens an existing removal operation without loading SSH defaults", async () => {
    show({
      registeredConnectorId: undefined,
      existingOperationId: "operation-1",
    } as never);
    expect(await screen.findByText("排空资源")).toBeVisible();
    expect(api.hosts.getRemovalOperation).toHaveBeenCalledWith("operation-1");
    expect(api.hosts.getRemovalConnectionDefaults).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("button", { name: "删除记录" }),
    ).not.toBeInTheDocument();
  });

  it.each([403, 404, 409, 500])(
    "keeps SSH fields hidden when loading defaults fails with HTTP %s",
    async (status) => {
      api.hosts.getRemovalConnectionDefaults.mockRejectedValue(
        new ApiError(
          {
            code: status === 403 ? "PERMISSION_DENIED" : "INTERNAL_ERROR",
            message_key: "errors.codes.INTERNAL_ERROR",
            request_id: `request-defaults-${status}`,
            retryable: status >= 500,
          },
          status,
        ),
      );
      show();
      expect(await screen.findByText("无法读取 SSH 连接信息")).toBeVisible();
      expect(screen.queryByLabelText("SSH 账号")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("SSH 凭据")).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "重试加载" })).toBeVisible();
      expect(api.secrets.listCredentials).not.toHaveBeenCalled();
    },
  );

  it("keeps SSH fields hidden after a network failure and allows retry", async () => {
    api.hosts.getRemovalConnectionDefaults.mockRejectedValue(
      new Error("network unavailable"),
    );
    show();
    expect(await screen.findByText("无法读取 SSH 连接信息")).toBeVisible();
    expect(screen.queryByLabelText("SSH 账号")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重试加载" })).toBeVisible();
    expect(api.secrets.listCredentials).not.toHaveBeenCalled();
  });

  it("retries loading defaults without asking for SSH input", async () => {
    api.hosts.getRemovalConnectionDefaults
      .mockRejectedValueOnce(new Error("network unavailable"))
      .mockResolvedValueOnce({
        status: "available",
        username: "install-admin",
        credential_id: "credential-1",
        credential_name: "SSH credential",
      });
    show();
    fireEvent.click(await screen.findByRole("button", { name: "重试加载" }));
    expect(await screen.findByText(/账号：install-admin/)).toBeVisible();
    expect(screen.queryByLabelText("SSH 账号")).not.toBeInTheDocument();
    expect(api.hosts.getRemovalConnectionDefaults).toHaveBeenCalledTimes(2);
  });

  it.each(["credential_unavailable", "unavailable"])(
    "shows editable SSH fields when defaults explicitly return %s",
    async (status) => {
      api.hosts.getRemovalConnectionDefaults.mockResolvedValue({
        status,
        username: "install-admin",
      });
      show();
      expect(
        await screen.findByRole("textbox", { name: /SSH 账号/ }),
      ).toHaveValue("install-admin");
      expect(
        screen.getByRole("button", { name: /SSH 凭据/ }),
      ).toBeInTheDocument();
    },
  );

  it("shows editable SSH fields only after the user requests a change", async () => {
    show();
    await screen.findByText(/账号：install-admin/);
    expect(screen.queryByLabelText("SSH 账号")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "更换账号或凭据" }));
    expect(screen.getByRole("textbox", { name: /SSH 账号/ })).toHaveValue(
      "install-admin",
    );
    expect(
      screen.getByRole("button", { name: /SSH 凭据/ }),
    ).toBeInTheDocument();
  });

  it("clears a preview when switching to another target", async () => {
    const view = show();
    await startSSH();
    await screen.findByText("Review removal action");
    view.rerenderTarget({
      ...target,
      id: "host-456",
      hostId: "host-456",
      name: "456",
      expectedVersion: 4,
      registeredConnectorId: "connector-456",
    } as RemovalTarget);
    expect(await screen.findByText(/账号：install-admin/)).toBeVisible();
    expect(screen.queryByText("Review removal action")).not.toBeInTheDocument();
  });

  it("clears a preview when the target resource version changes", async () => {
    const view = show();
    await startSSH();
    await screen.findByText("Review removal action");
    view.rerenderTarget({
      ...target,
      expectedVersion: target.expectedVersion + 1,
    });
    expect(await screen.findByText(/账号：install-admin/)).toBeVisible();
    expect(screen.queryByText("Review removal action")).not.toBeInTheDocument();
  });

  it("ignores a late SSH result after switching targets", async () => {
    let finish!: (value: unknown) => void;
    api.hosts.createConnectionTest.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const view = show();
    await startSSH();
    view.rerenderTarget({
      ...target,
      id: "host-456",
      hostId: "host-456",
      expectedVersion: 4,
      registeredConnectorId: "connector-456",
    });
    await act(async () => finish({ id: "late-test", status: "succeeded" }));
    expect(api.hosts.getConnectionTest).not.toHaveBeenCalled();
    expect(api.hosts.previewRemoval).not.toHaveBeenCalled();
  });

  it.each(["managed_host", "bastion_scope"] as const)(
    "removes an offline %s without SSH or credentials",
    async (type) => {
      // Reading installation defaults may itself be unavailable; record removal must still work.
      api.hosts.getRemovalConnectionDefaults.mockReturnValue(
        new Promise(() => {}),
      );
      show({ type });
      expect(
        screen.getByRole("button", { name: /^卸载机器上的 Argus 软件/ }),
      ).toHaveAttribute("aria-pressed", "true");
      fireEvent.click(screen.getByRole("button", { name: /^仅从 Argus 移除/ }));
      expect(screen.queryByText("SSH 自动卸载")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("SSH 账号")).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "生成移除预览" }));
      await screen.findByText("输入的资源名称不匹配。");
      expect(api.hosts.previewRemoval).not.toHaveBeenCalled();
      fireEvent.change(screen.getByLabelText(/输入 123 以确认/), {
        target: { value: "123" },
      });
      fireEvent.click(screen.getByRole("button", { name: "生成移除预览" }));
      await screen.findByText("Review removal action");
      expect(api.hosts.previewRemoval).toHaveBeenCalledWith({
        target_type: type,
        target_id: target.id,
        expected_version: 3,
        mode: "forget",
        confirmation_name: "123",
        connection_test_id: undefined,
        credential_id: undefined,
      });
      expect(api.hosts.createConnectionTest).not.toHaveBeenCalled();
      expect(api.hosts.getConnectionTest).not.toHaveBeenCalled();
    },
  );

  it("shows why record removal is disabled for an online resource", async () => {
    show({ connectionStatus: "online" });
    await screen.findByText(/账号：install-admin/);
    expect(
      screen.getByRole("button", { name: /^仅从 Argus 移除/ }),
    ).toBeDisabled();
    expect(screen.getByText("仅离线或卸载异常时可用")).toBeVisible();
  });

  it.each([
    ["TIMEOUT", "SSH 超时"],
    ["AUTH_FAILED", "SSH 身份验证失败"],
    ["CONNECTION_REFUSED", "拒绝 SSH 连接"],
    ["TARGET_UNROUTABLE", "无法通过网络访问"],
  ])(
    "explains %s and does not create an uninstall preview",
    async (code, message) => {
      api.hosts.getConnectionTest.mockResolvedValue({
        id: "test-1",
        status: "failed",
        error_code: code,
      });
      show();
      await startSSH();
      expect(
        await screen.findByText(
          new RegExp(`${message}.*尚未开始卸载`),
          {},
          { timeout: 3000 },
        ),
      ).toBeVisible();
      expect(api.hosts.getConnectionTest).toHaveBeenCalledTimes(1);
      expect(api.hosts.previewRemoval).not.toHaveBeenCalled();
      fireEvent.click(screen.getByRole("button", { name: /^仅从 Argus 移除/ }));
      expect(screen.queryByText("卸载准备失败")).not.toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "生成移除预览" }),
      ).toBeEnabled();
    },
  );

  it("requires fresh SSH evidence before normal uninstall", async () => {
    show();
    await startSSH();
    await screen.findByText("Review removal action", {}, { timeout: 3000 });
    expect(api.hosts.previewRemoval).toHaveBeenCalledWith(
      expect.objectContaining({
        mode: "uninstall",
        connection_test_id: "test-1",
        credential_id: "credential-1",
      }),
    );
  });

  it("stops polling and ignores a late SSH failure after changing the choice", async () => {
    let finish!: (value: unknown) => void;
    api.hosts.getConnectionTest.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    show();
    await startSSH();
    await waitFor(
      () => expect(api.hosts.getConnectionTest).toHaveBeenCalledTimes(1),
      { timeout: 3000 },
    );
    fireEvent.click(screen.getByRole("button", { name: /^仅从 Argus 移除/ }));
    fireEvent.change(screen.getByLabelText(/输入 123 以确认/), {
      target: { value: "123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "生成移除预览" }));
    await screen.findByText("Review removal action");
    await act(async () =>
      finish({ id: "test-1", status: "failed", error_code: "TIMEOUT" }),
    );
    expect(screen.queryByText("卸载准备失败")).not.toBeInTheDocument();
    expect(api.hosts.previewRemoval).toHaveBeenCalledTimes(1);
    expect(api.hosts.previewRemoval).toHaveBeenCalledWith(
      expect.objectContaining({ mode: "forget" }),
    );
    expect(api.hosts.getConnectionTest).toHaveBeenCalledTimes(1);
  });

  it("does not poll or preview after closing during test creation", async () => {
    let finish!: (value: unknown) => void;
    api.hosts.createConnectionTest.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    show();
    await startSSH();
    fireEvent.click(
      screen.getByRole("dialog").querySelector("button[slot='close']")!,
    );
    await act(async () => finish({ id: "test-1", status: "succeeded" }));
    expect(api.hosts.getConnectionTest).not.toHaveBeenCalled();
    expect(api.hosts.previewRemoval).not.toHaveBeenCalled();
  });

  it("bounds polling when the server keeps returning running", async () => {
    show();
    await screen.findByText(/账号：install-admin/);
    api.hosts.getConnectionTest.mockResolvedValue({
      id: "test-1",
      status: "running",
    });
    vi.useFakeTimers();
    fireEvent.click(screen.getByRole("button", { name: "生成卸载预览" }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(61_000);
    });
    expect(screen.getByText(/SSH 超时，尚未开始卸载/)).toBeVisible();
    const count = api.hosts.getConnectionTest.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(api.hosts.getConnectionTest).toHaveBeenCalledTimes(count);
    expect(api.hosts.previewRemoval).not.toHaveBeenCalled();
  });
});
