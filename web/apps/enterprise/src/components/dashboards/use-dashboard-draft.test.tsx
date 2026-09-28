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
afterEach(cleanup);
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
