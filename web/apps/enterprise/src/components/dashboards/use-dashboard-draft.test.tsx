// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ApiProvider,
  createMockApiClient,
  emptyDashboardSpec,
} from "@argus/api-client";
import type { ReactNode } from "react";
import { useDashboardDraft } from "./use-dashboard-draft";
import { newPanel, draftInput } from "./model";
afterEach(cleanup);
it("ends initial loading on a failed read and retries without replacing buffered editor changes", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const draft = await api.dashboards.createDraft({
    name: "retry",
    description: "",
    spec: emptyDashboardSpec(),
    proposed_bindings: [],
  });
  const read = vi
    .spyOn(api.dashboards, "draft")
    .mockRejectedValueOnce(new Error("offline"));
  const { result } = renderHook(() => useDashboardDraft(draft.id), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <ApiProvider client={api}>{children}</ApiProvider>
    ),
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.draft).toBeNull();
  expect(result.current.error).toBeInstanceOf(Error);
  act(() => result.current.reload());
  await waitFor(() => expect(result.current.draft?.name).toBe("retry"));
  expect(result.current.error).toBeUndefined();
  act(() =>
    result.current.update({ ...result.current.draft!, name: "buffered" }),
  );
  act(() => result.current.reload());
  expect(read).toHaveBeenCalledTimes(2);
  expect(result.current.draft?.name).toBe("buffered");
});
it("queues server mutations without overwriting a title edited while generated definitions are in flight", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const spec = emptyDashboardSpec();
  spec.panels = [newPanel(spec, "Initial title")];
  const draft = await api.dashboards.createDraft({
    name: "mutation",
    description: "",
    spec,
    proposed_bindings: [],
  });
  const { result } = renderHook(() => useDashboardDraft(draft.id), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <ApiProvider client={api}>{children}</ApiProvider>
    ),
  });
  await waitFor(() => expect(result.current.draft).toBeTruthy());
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let mutation!: Promise<unknown>;
  act(() => {
    mutation = result.current.mutate(async (stored) => {
      await gate;
      const updated = {
        ...stored,
        spec: {
          ...stored.spec,
          default_time_range: { kind: "relative" as const, seconds: 7200 },
        },
      };
      return api.dashboards.saveDraft(stored.id, {
        ...draftInput(updated),
        expected_version: stored.draft_version,
      });
    });
  });
  await act(async () => {
    await Promise.resolve();
  });
  act(() =>
    result.current.update({
      ...result.current.draft!,
      spec: {
        ...result.current.draft!.spec,
        panels: result.current.draft!.spec.panels.map((p) => ({
          ...p,
          title: "Typed while waiting",
        })),
      },
    }),
  );
  await act(async () => {
    release();
    await mutation;
  });
  expect(result.current.draft!.spec.panels[0]!.title).toBe(
    "Typed while waiting",
  );
  expect(result.current.draft!.spec.default_time_range.seconds).toBe(7200);
  expect(result.current.dirty).toBe(true);
  await act(async () => {
    await result.current.save();
  });
  const saved = await api.dashboards.draft(draft.id);
  expect(saved.spec.panels[0]!.title).toBe("Typed while waiting");
  expect(saved.spec.default_time_range.seconds).toBe(7200);
});
it("serializes overlapping saves and preserves edits made while a write is in flight", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  await api.auth.login({ username: "root", password: "123456" });
  const draft = await api.dashboards.createDraft({
    name: "initial",
    description: "",
    spec: emptyDashboardSpec(),
    proposed_bindings: [],
  });
  const original = api.dashboards.saveDraft;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let calls = 0;
  const save = vi
    .spyOn(api.dashboards, "saveDraft")
    .mockImplementation(async (...args) => {
      if (++calls === 1) await gate;
      return original(...args);
    });
  const { result } = renderHook(() => useDashboardDraft(draft.id), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <ApiProvider client={api}>{children}</ApiProvider>
    ),
  });
  await waitFor(() => expect(result.current.draft?.name).toBe("initial"));
  act(() => result.current.update({ ...result.current.draft!, name: "first" }));
  let first!: ReturnType<typeof result.current.save>;
  act(() => {
    first = result.current.save();
  });
  act(() =>
    result.current.update({ ...result.current.draft!, name: "latest" }),
  );
  await act(async () => {
    const second = result.current.save();
    release();
    await Promise.all([first, second]);
  });
  expect(save.mock.calls.map(([, input]) => input.expected_version)).toEqual([
    1, 2,
  ]);
  expect(result.current.draft?.name).toBe("latest");
  expect(result.current.dirty).toBe(false);
  expect((await api.dashboards.draft(draft.id)).name).toBe("latest");
});
