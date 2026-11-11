// @vitest-environment jsdom
import { StrictMode, type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import {
  ApiProvider,
  createMockApiClient,
  type KubernetesLogSession,
  type KubernetesLogSessionInput,
} from "@argus/api-client";
import { usePodLogSession } from "./use-pod-log-session";
const input: KubernetesLogSessionInput = {
  namespace_id: "ns",
  pod: "api",
  pod_uid: "pod-uid",
  container: "main",
  previous: false,
  tail_lines: 100,
  since_seconds: 300,
};
const session: KubernetesLogSession = {
  id: "old",
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
const ticket = (id: string) => ({
  session: { ...session, id },
  ticket: "private-ticket",
  ticket_expires_at: new Date(Date.now() + 30000).toISOString(),
});
const batch = (content: string, complete = false) => ({
  content,
  complete,
  next_cursor: "cursor",
  observed_at: new Date().toISOString(),
  pending_line: false,
  truncated: false,
  redacted: true,
});
const formatError = (cause: unknown) => (cause as Error).message;

it("does not orphan a session when closed while automatic creation is pending", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  let resolve!: (value: ReturnType<typeof ticket>) => void;
  const create = vi
    .spyOn(api.kubernetesWorkbench, "createLogSession")
    .mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
  const attach = vi.spyOn(api.kubernetesWorkbench, "attachLogSession");
  const close = vi
    .spyOn(api.kubernetesWorkbench, "closeLogSession")
    .mockResolvedValue({ ...session, status: "closed" });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StrictMode>
      <ApiProvider client={api}>{children}</ApiProvider>
    </StrictMode>
  );
  const view = renderHook(
    () => usePodLogSession("cluster", input, formatError),
    { wrapper },
  );
  await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
  view.unmount();
  await act(async () => resolve(ticket("old")));
  expect(close).toHaveBeenCalledExactlyOnceWith("cluster", "old");
  expect(attach).not.toHaveBeenCalled();
});

it("ignores a late response from the previous Pod and closes each owned session", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  vi.spyOn(api.kubernetesWorkbench, "createLogSession")
    .mockResolvedValueOnce(ticket("old"))
    .mockResolvedValueOnce(ticket("new"));
  vi.spyOn(api.kubernetesWorkbench, "attachLogSession").mockImplementation(
    async (_cluster, id) => ({ ...session, id }),
  );
  let resolve!: (value: ReturnType<typeof batch>) => void;
  vi.spyOn(api.kubernetesWorkbench, "readLogSession").mockImplementation(
    async (_cluster, id) => {
      if (id === "old")
        return new Promise((done) => {
          resolve = done;
        });
      return batch("current Pod");
    },
  );
  const close = vi
    .spyOn(api.kubernetesWorkbench, "closeLogSession")
    .mockResolvedValue({ ...session, status: "closed" });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <ApiProvider client={api}>{children}</ApiProvider>
  );
  const view = renderHook(
    ({ value }) => usePodLogSession("cluster", value, formatError),
    { wrapper, initialProps: { value: input } },
  );
  await waitFor(() => expect(resolve).toBeDefined());
  view.rerender({
    value: { ...input, pod: "replacement", pod_uid: "replacement-uid" },
  });
  await waitFor(() => expect(view.result.current.content).toBe("current Pod"));
  await act(async () => resolve(batch("stale Pod")));
  expect(view.result.current.content).toBe("current Pod");
  expect(close).toHaveBeenCalledWith("cluster", "old");
  view.unmount();
  expect(close).toHaveBeenCalledWith("cluster", "new");
});

it("continues authorization checks after bounded output and clears content on denial", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  vi.spyOn(api.kubernetesWorkbench, "createLogSession").mockResolvedValue(
    ticket("old"),
  );
  vi.spyOn(api.kubernetesWorkbench, "attachLogSession").mockResolvedValue(
    session,
  );
  vi.spyOn(api.kubernetesWorkbench, "readLogSession").mockResolvedValue(
    batch("sensitive existing output", true),
  );
  vi.spyOn(api.kubernetesWorkbench, "getLogSession").mockRejectedValue(
    new Error("access revoked"),
  );
  vi.spyOn(api.kubernetesWorkbench, "closeLogSession").mockResolvedValue({
    ...session,
    status: "closed",
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <ApiProvider client={api}>{children}</ApiProvider>
  );
  const view = renderHook(
    () => usePodLogSession("cluster", input, formatError),
    { wrapper },
  );
  await waitFor(() =>
    expect(view.result.current.content).toBe("sensitive existing output"),
  );
  await waitFor(
    () => expect(view.result.current.error).toBe("access revoked"),
    { timeout: 4000 },
  );
  expect(view.result.current.content).toBe("");
  view.unmount();
});
