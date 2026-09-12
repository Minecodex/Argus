// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Host, HostOnboardingOperation } from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";

import "../../i18n";
import { HostOnboardingProgress } from "./host-onboarding-progress";

const api = vi.hoisted(() => ({
  connectors: { listBastionScopes: vi.fn() },
  hosts: { getOnboardingOperation: vi.fn() },
}));

vi.mock("@argus/api-client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@argus/api-client")>()),
  useApi: () => api,
}));

const messages = {
  HOST_ONBOARDING_CALLBACK_CONFIG_INVALID:
    "平台回调地址配置无效，请联系平台管理员检查安装配置。",
  HOST_ONBOARDING_CALLBACK_DNS_FAILED:
    "目标主机无法解析平台回调地址，请检查所选接入路径的 DNS 与域名配置。",
  HOST_ONBOARDING_CALLBACK_CONNECT_FAILED:
    "目标主机无法通过所选接入路径连接平台回调地址，请检查路由、防火墙及中继或隧道状态。",
  HOST_ONBOARDING_CALLBACK_TLS_FAILED:
    "回调 TLS 校验失败，请检查证书、SNI 与信任链。",
  HOST_ONBOARDING_CALLBACK_HTTP_FAILED:
    "回调接口返回异常，请检查入口路由与服务状态。",
  HOST_ONBOARDING_CALLBACK_RESPONSE_INVALID:
    "回调响应格式无效，请检查服务版本与入口转发。",
  HOST_ONBOARDING_CALLBACK_TIMEOUT:
    "等待回调响应超时，请检查网络连通性与服务负载。",
} as const;

function hostWithFailure(code: string): Host {
  return {
    id: "00000000-0000-0000-0000-000000000101",
    enterprise_id: "00000000-0000-0000-0000-000000000001",
    name: "callback-host",
    address: "192.0.2.10",
    port: 22,
    platform: "linux",
    role: "managed_host",
    control_path: "executor_tunnel",
    environment: "development",
    labels: {},
    labels_version: 1,
    resource_version: 1,
    connection_status: "onboarding",
    status: "active",
    onboarding: {
      state: "install_failed",
      install_method: "ssh",
      operation_id: "00000000-0000-0000-0000-000000000201",
      error_code: code,
      updated_at: "2026-09-11T00:01:00Z",
    },
    created_at: "2026-09-11T00:00:00Z",
    updated_at: "2026-09-11T00:01:00Z",
  };
}

function operationWithFailure(code: string): HostOnboardingOperation {
  return {
    id: "00000000-0000-0000-0000-000000000201",
    host_id: "00000000-0000-0000-0000-000000000101",
    connector_id: "00000000-0000-0000-0000-000000000301",
    install_method: "ssh",
    ssh_path: "direct_executor",
    target_platform: "linux_amd64",
    control_path: "executor_tunnel",
    stage: "enrolling",
    status: "failed",
    attempts: 1,
    max_attempts: 3,
    error_code: code,
    events: [
      {
        id: "00000000-0000-0000-0000-000000000401",
        stage: "enrolling",
        status: "failed",
        error_code: code,
        occurred_at: "2026-09-11T00:01:00Z",
      },
    ],
    expires_at: "2026-09-11T00:30:00Z",
    created_at: "2026-09-11T00:00:00Z",
    updated_at: "2026-09-11T00:01:00Z",
  };
}

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <LocaleProvider>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </LocaleProvider>
  );
}

beforeEach(() => {
  vi.resetAllMocks();
  window.localStorage.setItem("argus.locale", "zh-CN");
  api.connectors.listBastionScopes.mockResolvedValue({ items: [] });
});
afterEach(cleanup);

describe("HostOnboardingProgress callback failures", () => {
  for (const [code, message] of Object.entries(messages)) {
    it(`explains ${code} while retaining its technical code`, async () => {
      api.hosts.getOnboardingOperation.mockResolvedValue(
        operationWithFailure(code),
      );
      render(<HostOnboardingProgress host={hostWithFailure(code)} />, {
        wrapper,
      });

      expect(screen.getByText(message)).toBeVisible();
      expect(screen.getByText(`错误码：${code}`)).toBeVisible();
      fireEvent.click(screen.getByRole("button", { name: "查看安装进度" }));

      const dialog = await screen.findByRole("dialog", {
        name: "查看安装进度",
      });
      expect(await within(dialog).findAllByText(message)).toHaveLength(2);
      expect(
        await within(dialog).findAllByText(`错误码：${code}`),
      ).toHaveLength(2);
    });
  }
});
