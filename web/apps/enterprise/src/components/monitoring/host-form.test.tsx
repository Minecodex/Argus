// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { afterAll, afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  ApiProvider,
  createMockApiClient,
  type MonitoringCollection,
} from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import "../../i18n";
import { MonitoringHostForm } from "./host-form";

let canManage = true;
vi.mock("../../lib/permissions", () => ({ usePermission: () => canManage }));
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);
const originalAnimations = Element.prototype.getAnimations;
Element.prototype.getAnimations = () => [];
afterEach(() => {
  cleanup();
  canManage = true;
  localStorage.clear();
});
afterAll(() => {
  vi.unstubAllGlobals();
  if (originalAnimations) Element.prototype.getAnimations = originalAnimations;
  else Reflect.deleteProperty(Element.prototype, "getAnimations");
});

const collection: MonitoringCollection = {
  id: "c8719254-85e7-48a2-bbe0-3b4191fde23c",
  integration: "mysql",
  name: "Orders database",
  target: {
    kind: "monitoring_target",
    id: "c034c7c9-81dc-419e-9abe-5a96cfd183c0",
  },
  collector_id: "8f8c36d7-1953-47d0-93fb-83c6cd0492eb",
  version: 7,
  assignment_epoch: 1,
  config: { interval: "30s", timeout: "10s" },
  desired_enabled: true,
  desired_revision: 1,
  effective_revision: 1,
  apply_status: "effective",
  credential_configured: true,
  status: "active",
  health: "healthy",
  freshness: "fresh",
  error_code: "",
};

for (const mode of ["preview", "revoked", "stale"] as const) {
  it(`keeps sensitive credential entry in the trusted host: ${mode}`, async () => {
    const api = createMockApiClient({ persist: false, delay: 0 });
    await api.auth.login({ username: "root", password: "123456" });
    vi.spyOn(api.monitoring, "collection").mockResolvedValue(
      structuredClone(collection),
    );
    const nativePreview = api.monitoring.preview;
    const preview = vi
      .spyOn(api.monitoring, "preview")
      .mockImplementation(async () => {
        if (mode === "stale") throw new Error("Version conflict");
        return nativePreview({
          kind: "application",
          operation: "create",
          application: { name: "Form preview fixture", environment: "testing" },
        });
      });
    const cache = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const data = {
      tool: "monitoring.collection.form",
      schema_version: "argus.monitoring_host_form/v1",
      integration: "mysql",
      collection_id: collection.id,
    };
    const tree = () => (
      <LocaleProvider>
        <ApiProvider client={api}>
          <QueryClientProvider client={cache}>
            <MonitoringHostForm data={data} />
          </QueryClientProvider>
        </ApiProvider>
      </LocaleProvider>
    );
    const view = render(tree());
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "打开监控安全表单" }),
      ).toBeEnabled(),
    );
    fireEvent.click(screen.getByRole("button", { name: "打开监控安全表单" }));
    await screen.findByRole("dialog", { name: "更新监控认证" });
    fireEvent.change(screen.getByLabelText(/^用户名/), {
      target: { value: "rotated-monitor" },
    });
    fireEvent.change(screen.getByLabelText(/^密码/), {
      target: { value: "private-host-form-canary" },
    });
    if (mode === "revoked") {
      canManage = false;
      view.rerender(tree());
      await waitFor(() =>
        expect(
          screen.queryByRole("dialog", { name: "更新监控认证" }),
        ).not.toBeInTheDocument(),
      );
      expect(preview).not.toHaveBeenCalled();
    } else {
      fireEvent.click(screen.getByRole("button", { name: "预览采集配置" }));
      await waitFor(() => expect(preview).toHaveBeenCalledTimes(1));
      expect(preview.mock.calls[0]?.[0]).toMatchObject({
        expected_version: 7,
        id: collection.id,
        collection: {
          credential_update: "replace",
          collector_id: collection.collector_id,
        },
        credentials: {
          username: "rotated-monitor",
          password: "private-host-form-canary",
        },
      });
      if (mode === "stale") {
        await screen.findByRole("alert");
        expect(
          screen.getByRole("dialog", { name: "更新监控认证" }),
        ).toBeInTheDocument();
      } else {
        await waitFor(() =>
          expect(screen.queryByLabelText(/^密码/)).not.toBeInTheDocument(),
        );
      }
    }
    expect(JSON.stringify(data)).not.toContain("private-host-form-canary");
    expect(JSON.stringify(localStorage)).not.toContain(
      "private-host-form-canary",
    );
  });
}
