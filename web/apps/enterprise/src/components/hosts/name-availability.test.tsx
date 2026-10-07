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
import { ApiError, type BastionScope } from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import i18n from "../../i18n";
import { AddHostWizard } from "./add-host-wizard";
import { AddBastionDialog } from "./add-bastion-dialog";
import type { Host, PendingActionPublic } from "@argus/api-client";

const api = vi.hoisted(() => ({
  hosts: {
    checkNameAvailability: vi.fn(),
    createConnectionTest: vi.fn(),
    previewCreateResource: vi.fn(),
    previewRetryResource: vi.fn(),
  },
  connectors: {
    checkBastionNameAvailability: vi.fn(),
    previewCreateBastionScope: vi.fn(),
  },
  secrets: { listCredentials: vi.fn() },
  approvals: { cancel: vi.fn(), confirm: vi.fn() },
}));
vi.mock("@argus/api-client", async (original) => ({
  ...(await original<typeof import("@argus/api-client")>()),
  useApi: () => api,
}));
const scope = {
  id: "scope-ready",
  name: "bastion",
  relay_status: "ready",
  relay_address: "10.0.0.1",
  relay_https_port: 8445,
} as BastionScope;
const modes = [
  ["host", "主机可访问 Argus · 一行命令"],
  ["host", "平台可 SSH · 主机可访问 Argus"],
  ["host", "平台可 SSH · 主机无出站"],
  ["host", "主机可访问堡垒机 · 一行命令"],
  ["host", "堡垒机可 SSH 成员主机"],
  ["bastion", "堡垒机可出站访问 Argus"],
  ["bastion", "平台代装（双向可达）"],
  ["bastion", "平台代装 + 控制隧道（堡垒机无出站）"],
] as const;

beforeEach(async () => {
  vi.resetAllMocks();
  HTMLElement.prototype.scrollIntoView = vi.fn();
  window.localStorage.setItem("argus.locale", "zh-CN");
  await i18n.changeLanguage("zh-CN");
  api.hosts.checkNameAvailability.mockResolvedValue({ available: false });
  api.connectors.checkBastionNameAvailability.mockResolvedValue({
    available: false,
  });
  api.secrets.listCredentials.mockResolvedValue([]);
});
afterEach(cleanup);

function show(kind: "host" | "bastion", open = true, retryHost?: Host) {
  const child =
    kind === "host" ? (
      <AddHostWizard
        open={open}
        onOpenChange={vi.fn()}
        onCreated={vi.fn()}
        scopes={[scope]}
        retryHost={retryHost}
      />
    ) : (
      <AddBastionDialog
        open={open}
        onOpenChange={vi.fn()}
        onCreated={vi.fn()}
      />
    );
  return (
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider>{child}</LocaleProvider>
    </QueryClientProvider>
  );
}
function nameInput() {
  return screen.getByRole("textbox", { name: /主机名|名称/ });
}
function nameCheck(kind: "host" | "bastion") {
  return kind === "host"
    ? api.hosts.checkNameAvailability
    : api.connectors.checkBastionNameAvailability;
}
function next() {
  fireEvent.click(screen.getByRole("button", { name: "下一步" }));
}

const pendingAction: PendingActionPublic = {
  schema_version: "argus.pending_action/v1",
  action_ref: "create-ref",
  action_type: "host.create",
  title: "Create",
  summary: "Create",
  risk: "write",
  preview: { name: "taken" },
  diff: [],
  status: "awaiting_confirmation",
  available_actions: ["confirm", "cancel"],
  expires_at: new Date(Date.now() + 600_000).toISOString(),
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

describe("creation name checks", () => {
  it.each([
    ["host", "平台可 SSH · 主机可访问 Argus", "direct", false],
    ["host", "平台可 SSH · 主机无出站", "executor_tunnel", false],
    ["host", "堡垒机可 SSH 成员主机", "bastion_relay", false],
    ["bastion", "平台代装（双向可达）", "direct", false],
    [
      "bastion",
      "平台代装 + 控制隧道（堡垒机无出站）",
      "executor_tunnel",
      false,
    ],
    ["host", "平台可 SSH · 主机无出站", "executor_tunnel", true],
  ] as const)(
    "blocks %s callback failure and freezes %s path (%s, retry %s)",
    async (kind, label, path, retry) => {
      nameCheck(kind).mockResolvedValue({ available: true });
      api.secrets.listCredentials.mockResolvedValue([
        {
          id: "ssh-1",
          name: "SSH test key",
          protocol: "ssh",
          status: "active",
        },
      ]);
      api.hosts.createConnectionTest.mockResolvedValue({
        id: "callback-failed",
        status: "failed",
        error_code: "HOST_ONBOARDING_CALLBACK_DNS_FAILED",
        checks: [
          { name: "authentication", status: "passed" },
          {
            name: "onboarding_callback",
            status: "failed",
            detail: "HOST_ONBOARDING_CALLBACK_DNS_FAILED",
          },
        ],
      });
      render(
        show(
          kind,
          true,
          retry
            ? ({
                id: "existing-host",
                name: "existing-host",
                control_path: path,
                platform: "linux",
                environment: "production",
              } as Host)
            : undefined,
        ),
      );
      fireEvent.click(
        screen.getByRole("button", {
          name: new RegExp(`^${label.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`),
        }),
      );
      next();
      fireEvent.change(nameInput(), { target: { value: "callback-host" } });
      fireEvent.change(screen.getByRole("textbox", { name: "地址" }), {
        target: { value: "192.0.2.10" },
      });
      fireEvent.change(screen.getByRole("textbox", { name: "登录账号" }), {
        target: { value: "root" },
      });
      if (path === "bastion_relay") {
        fireEvent.click(screen.getByRole("button", { name: "所属堡垒机" }));
        fireEvent.click(await screen.findByRole("option", { name: /bastion/ }));
      }
      await waitFor(() =>
        expect(
          screen.getByRole("button", {
            name: kind === "host" ? "SSH 凭据" : "登录凭据或密钥",
          }),
        ).not.toBeDisabled(),
      );
      fireEvent.click(
        screen.getByRole("button", {
          name: kind === "host" ? "SSH 凭据" : "登录凭据或密钥",
        }),
      );
      fireEvent.click(
        await screen.findByRole("option", { name: "SSH test key" }),
      );
      fireEvent.submit(nameInput().closest("form")!);
      await waitFor(() =>
        expect(api.hosts.createConnectionTest).toHaveBeenCalledWith(
          expect.objectContaining({ onboarding_control_path: path }),
        ),
      );
      expect(
        (await screen.findAllByText(/目标主机无法解析平台回调地址/)).length,
      ).toBeGreaterThan(0);
      expect(
        screen.getAllByText(/HOST_ONBOARDING_CALLBACK_DNS_FAILED/).length,
      ).toBeGreaterThan(0);
      expect(screen.getByText(/SSH 身份验证/)).toBeVisible();
      expect(api.hosts.previewCreateResource).not.toHaveBeenCalled();
      expect(api.hosts.previewRetryResource).not.toHaveBeenCalled();
      if (retry) expect(api.hosts.checkNameAvailability).not.toHaveBeenCalled();
      expect(api.connectors.previewCreateBastionScope).not.toHaveBeenCalled();
      expect(
        screen.queryByRole("button", { name: "确认执行" }),
      ).not.toBeInTheDocument();
    },
  );

  it.each(["host", "bastion"] as const)(
    "accepts 128 Unicode characters in %s names",
    async (kind) => {
      nameCheck(kind).mockResolvedValue({ available: true });
      const preview =
        kind === "host"
          ? api.hosts.previewCreateResource
          : api.connectors.previewCreateBastionScope;
      preview.mockResolvedValue(pendingAction);
      render(show(kind));
      next();
      fireEvent.change(nameInput(), { target: { value: "😀".repeat(128) } });
      fireEvent.submit(nameInput().closest("form")!);
      await screen.findByRole("button", { name: "确认执行" });
      expect(preview).toHaveBeenCalled();
    },
  );

  it.each(["host", "bastion"] as const)(
    "ignores an old %s preview conflict after rename and reopen",
    async (kind) => {
      nameCheck(kind).mockResolvedValue({ available: true });
      const preview =
        kind === "host"
          ? api.hosts.previewCreateResource
          : api.connectors.previewCreateBastionScope;
      let reject!: (reason: unknown) => void;
      preview.mockImplementation(
        () =>
          new Promise((_, fail) => {
            reject = fail;
          }),
      );
      const view = render(show(kind));
      next();
      fireEvent.change(nameInput(), { target: { value: "first-name" } });
      fireEvent.submit(nameInput().closest("form")!);
      await waitFor(() => expect(preview).toHaveBeenCalledTimes(1));
      fireEvent.change(nameInput(), { target: { value: "second-name" } });
      const conflict = new ApiError(
        {
          code: "RESOURCE_NAME_CONFLICT",
          message_key: "errors.common.resource_name_conflict",
          request_id: "old-preview",
          retryable: false,
        },
        409,
      );
      await act(async () => reject(conflict));
      expect(nameInput()).not.toHaveAttribute("aria-invalid", "true");
      fireEvent.submit(nameInput().closest("form")!);
      await waitFor(() => expect(preview).toHaveBeenCalledTimes(2));
      view.rerender(show(kind, false));
      view.rerender(show(kind, true));
      await act(async () => reject(conflict));
      expect(nameInput()).not.toHaveAttribute("aria-invalid", "true");
      expect(screen.queryByText(/old-preview/)).not.toBeInTheDocument();
    },
  );
  it.each(modes)(
    "reports an inline duplicate on blur for %s / %s",
    async (kind, mode) => {
      render(show(kind));
      fireEvent.click(
        screen.getByRole("button", {
          name: new RegExp(`^${mode.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`),
        }),
      );
      next();
      fireEvent.change(nameInput(), { target: { value: "taken" } });
      fireEvent.blur(nameInput());
      await screen.findByText("该名称已被未删除的资源使用，请更换名称后重试。");
      expect(nameInput()).toHaveAttribute("aria-invalid", "true");
      expect(api.hosts.createConnectionTest).not.toHaveBeenCalled();
      expect(api.hosts.previewCreateResource).not.toHaveBeenCalled();
      expect(api.connectors.previewCreateBastionScope).not.toHaveBeenCalled();
    },
  );

  it.each(["host", "bastion"] as const)(
    "checks %s command submit and rechecks a previous available result",
    async (kind) => {
      nameCheck(kind)
        .mockResolvedValueOnce({ available: true })
        .mockResolvedValue({ available: false });
      render(show(kind));
      next();
      fireEvent.change(nameInput(), { target: { value: "taken-now" } });
      fireEvent.blur(nameInput());
      await waitFor(() => expect(nameCheck(kind)).toHaveBeenCalledTimes(1));
      await act(async () => {});
      fireEvent.submit(nameInput().closest("form")!);
      await screen.findByText("该名称已被未删除的资源使用，请更换名称后重试。");
      expect(nameCheck(kind)).toHaveBeenCalledTimes(2);
      expect(api.hosts.previewCreateResource).not.toHaveBeenCalled();
      expect(api.connectors.previewCreateBastionScope).not.toHaveBeenCalled();
    },
  );

  it("ignores stale results when the name changes or the dialog closes", async () => {
    let resolve!: (result: { available: boolean }) => void;
    api.hosts.checkNameAvailability.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const view = render(show("host"));
    next();
    fireEvent.change(nameInput(), { target: { value: "old-name" } });
    fireEvent.blur(nameInput());
    await waitFor(() =>
      expect(api.hosts.checkNameAvailability).toHaveBeenCalled(),
    );
    fireEvent.change(nameInput(), { target: { value: "new-name" } });
    await act(async () => resolve({ available: false }));
    expect(
      screen.queryByText("该名称已被未删除的资源使用，请更换名称后重试。"),
    ).not.toBeInTheDocument();
    fireEvent.blur(nameInput());
    view.rerender(show("host", false));
    await act(async () => resolve({ available: false }));
    view.rerender(show("host", true));
    expect(
      screen.queryByText("该名称已被未删除的资源使用，请更换名称后重试。"),
    ).not.toBeInTheDocument();
  });

  it.each(["host", "bastion"] as const)(
    "maps authoritative %s preview conflicts to the name field",
    async (kind) => {
      nameCheck(kind).mockResolvedValue({ available: true });
      const preview =
        kind === "host"
          ? api.hosts.previewCreateResource
          : api.connectors.previewCreateBastionScope;
      preview.mockRejectedValue(
        new ApiError(
          {
            code: "RESOURCE_NAME_CONFLICT",
            message_key: "errors.common.resource_name_conflict",
            request_id: "conflict-req",
            retryable: false,
          },
          409,
        ),
      );
      render(show(kind));
      next();
      fireEvent.change(nameInput(), { target: { value: "racing-name" } });
      fireEvent.submit(nameInput().closest("form")!);
      await screen.findByText(
        /该名称已被未删除的资源使用，请更换名称后重试。.*conflict-req/,
      );
      expect(nameInput()).toHaveAttribute("aria-invalid", "true");
    },
  );

  it.each(["host", "bastion"] as const)(
    "returns %s confirmation conflicts to the editable name field",
    async (kind) => {
      nameCheck(kind).mockResolvedValue({ available: true });
      const preview =
        kind === "host"
          ? api.hosts.previewCreateResource
          : api.connectors.previewCreateBastionScope;
      preview.mockResolvedValue(pendingAction);
      api.approvals.confirm.mockRejectedValue(
        new ApiError(
          {
            code: "RESOURCE_NAME_CONFLICT",
            message_key: "errors.common.resource_name_conflict",
            request_id: "commit-req",
            retryable: false,
          },
          409,
        ),
      );
      render(show(kind));
      next();
      fireEvent.change(nameInput(), { target: { value: "taken" } });
      fireEvent.submit(nameInput().closest("form")!);
      fireEvent.click(await screen.findByRole("button", { name: "确认执行" }));
      await screen.findByText(
        /该名称已被未删除的资源使用，请更换名称后重试。.*commit-req/,
      );
      expect(nameInput()).toHaveValue("taken");
      expect(nameInput()).toHaveAttribute("aria-invalid", "true");
    },
  );

  it("shares an in-flight blur check with submit and fails closed on network errors", async () => {
    let reject!: (reason: unknown) => void;
    api.hosts.checkNameAvailability.mockImplementation(
      () =>
        new Promise((_, fail) => {
          reject = fail;
        }),
    );
    render(show("host"));
    next();
    fireEvent.change(nameInput(), { target: { value: "network-check" } });
    fireEvent.blur(nameInput());
    fireEvent.submit(nameInput().closest("form")!);
    await screen.findByText("正在检查名称…");
    await act(async () => reject(new Error("secret internal failure")));
    await screen.findByText("暂时无法检查名称，请重试。");
    expect(api.hosts.checkNameAvailability).toHaveBeenCalledTimes(1);
    expect(api.hosts.previewCreateResource).not.toHaveBeenCalled();
    expect(
      screen.queryByText(/secret internal failure/),
    ).not.toBeInTheDocument();
  });

  it("does not reject an installation retry because its existing name is occupied", async () => {
    const retryHost = {
      id: "existing",
      name: "existing-name",
      control_path: "direct",
      platform: "linux",
      environment: "production",
    } as Host;
    api.hosts.previewRetryResource.mockResolvedValue(pendingAction);
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LocaleProvider>
          <AddHostWizard
            open
            onOpenChange={vi.fn()}
            onCreated={vi.fn()}
            scopes={[scope]}
            retryHost={retryHost}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    fireEvent.click(
      screen.getByRole("button", { name: /^主机可访问 Argus · 一行命令/ }),
    );
    next();
    fireEvent.blur(nameInput());
    fireEvent.submit(nameInput().closest("form")!);
    await screen.findByRole("button", { name: "确认执行" });
    expect(api.hosts.checkNameAvailability).not.toHaveBeenCalled();
    expect(api.hosts.previewRetryResource).toHaveBeenCalled();
  });
});
