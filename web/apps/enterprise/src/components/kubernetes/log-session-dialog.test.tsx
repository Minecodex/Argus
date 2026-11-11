// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterAll, afterEach, expect, it, vi } from "vitest";
import {
  ApiProvider,
  createMockApiClient,
  type KubernetesInventoryObject,
  type KubernetesLogSession,
} from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";
import "../../i18n";
import { KubernetesLogSessionDialog } from "./log-session-dialog";
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);
const previousAnimations = Element.prototype.getAnimations;
Element.prototype.getAnimations = () => [];
afterEach(cleanup);
afterAll(() => {
  vi.unstubAllGlobals();
  if (previousAnimations) Element.prototype.getAnimations = previousAnimations;
  else Reflect.deleteProperty(Element.prototype, "getAnimations");
});
const object: KubernetesInventoryObject = {
  definition: {
    key: "pod",
    group: "",
    version: "v1",
    resource: "pods",
    kind: "Pod",
    namespaced: true,
    category: "workloads",
    manage_permission: "kubernetes.workload.manage",
  },
  uid: "pod-uid",
  namespace: "orders",
  namespace_uid: "ns-uid",
  name: "api",
  resource_version: "1",
  generation: 1,
  labels: {},
  owners: [],
  created_at: new Date().toISOString(),
  observed_at: new Date().toISOString(),
  redacted: false,
  data: {
    apiVersion: "v1",
    kind: "Pod",
    metadata: { name: "api", namespace: "orders", uid: "pod-uid" },
    spec: { containers: [{ name: "main" }] },
    status: {
      containerStatuses: [
        { name: "main", restartCount: 0, containerID: "containerd://current" },
      ],
    },
  },
};
it("clears frozen log content when access is denied, even while the reader paused", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  const session: KubernetesLogSession = {
    id: "session",
    cluster_id: "cluster",
    namespace_id: "ns",
    namespace: "orders",
    namespace_uid: "ns-uid",
    pod: "api",
    pod_uid: "pod-uid",
    container: "main",
    previous: false,
    restart_count: 0,
    status: "active",
    version: 2,
    expires_at: new Date(Date.now() + 60000).toISOString(),
  };
  vi.spyOn(api.kubernetesWorkbench, "createLogSession").mockResolvedValue({
    session,
    ticket: "private-ticket",
    ticket_expires_at: new Date(Date.now() + 30000).toISOString(),
  });
  vi.spyOn(api.kubernetesWorkbench, "attachLogSession").mockResolvedValue(
    session,
  );
  vi.spyOn(api.kubernetesWorkbench, "readLogSession").mockResolvedValue({
    content: "private container line\n",
    next_cursor: "",
    observed_at: new Date().toISOString(),
    complete: true,
    truncated: false,
    pending_line: false,
    redacted: true,
  });
  vi.spyOn(api.kubernetesWorkbench, "getLogSession").mockRejectedValue(
    new Error("access revoked"),
  );
  const close = vi
    .spyOn(api.kubernetesWorkbench, "closeLogSession")
    .mockResolvedValue({ ...session, status: "closed" });
  render(
    <LocaleProvider locale="zh-CN">
      <ApiProvider client={api}>
        <KubernetesLogSessionDialog
          clusterId="cluster"
          namespaceId="ns"
          object={object}
          onClose={() => {}}
        />
      </ApiProvider>
    </LocaleProvider>,
  );
  await waitFor(() =>
    expect(screen.getByRole("log")).toHaveTextContent("private container line"),
  );
  fireEvent.click(screen.getByRole("button", { name: "暂停" }));
  await waitFor(() => expect(screen.getByRole("alert")).toBeVisible(), {
    timeout: 4000,
  });
  expect(screen.getByRole("log")).not.toHaveTextContent(
    "private container line",
  );
  expect(close).toHaveBeenCalledExactlyOnceWith("cluster", "session");
});
