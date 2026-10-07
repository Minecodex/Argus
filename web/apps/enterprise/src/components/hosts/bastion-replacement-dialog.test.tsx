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
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "@argus/ui";
import type { BastionScope, Host } from "@argus/api-client";
import i18n from "../../i18n";
import { BastionReplacementDialog } from "./bastion-replacement-dialog";

const api = vi.hoisted(() => ({
  hosts: { createConnectionTest: vi.fn() },
  connectors: { previewConnectorReplacement: vi.fn() },
  secrets: { listCredentials: vi.fn() },
}));
vi.mock("@argus/api-client", async (original) => ({
  ...(await original<typeof import("@argus/api-client")>()),
  useApi: () => api,
}));

beforeEach(async () => {
  vi.resetAllMocks();
  HTMLElement.prototype.scrollIntoView = vi.fn();
  await i18n.changeLanguage("zh-CN");
  api.secrets.listCredentials.mockResolvedValue([
    { id: "ssh-1", name: "SSH test key", protocol: "ssh", status: "active" },
  ]);
  api.hosts.createConnectionTest.mockResolvedValue({
    id: "test-failed",
    status: "failed",
    error_code: "HOST_ONBOARDING_CALLBACK_TLS_FAILED",
    checks: [
      { name: "authentication", status: "passed" },
      { name: "onboarding_callback", status: "failed" },
    ],
  });
});
afterEach(cleanup);

it.each([
  ["direct_install", "direct"],
  ["direct_install_tunnel", "executor_tunnel"],
] as const)(
  "requires %s callback proof before replacing the Connector",
  async (mode, path) => {
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider>
          <BastionReplacementDialog
            scope={
              {
                id: "scope-1",
                onboarding_mode: mode,
                resource_version: 3,
              } as BastionScope
            }
            host={{ id: "host-1", address: "192.0.2.10", port: 22 } as Host}
            onChanged={vi.fn()}
            onOpenChange={vi.fn()}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    fireEvent.change(screen.getByRole("textbox", { name: "登录账号" }), {
      target: { value: "root" },
    });
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "SSH 凭据" }),
      ).not.toBeDisabled(),
    );
    fireEvent.click(screen.getByRole("button", { name: "SSH 凭据" }));
    fireEvent.click(
      await screen.findByRole("option", { name: "SSH test key" }),
    );
    fireEvent.submit(
      screen.getByRole("textbox", { name: "地址" }).closest("form")!,
    );
    await waitFor(() =>
      expect(api.hosts.createConnectionTest).toHaveBeenCalledWith(
        expect.objectContaining({ onboarding_control_path: path }),
      ),
    );
    expect(
      await screen.findByText(/HOST_ONBOARDING_CALLBACK_TLS_FAILED/),
    ).toBeVisible();
    expect(screen.getByText(/回调 TLS 校验失败/)).toBeVisible();
    expect(screen.getByText("SSH 身份验证: 通过")).toBeVisible();
    expect(api.connectors.previewConnectorReplacement).not.toHaveBeenCalled();
  },
);
