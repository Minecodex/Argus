// @vitest-environment jsdom
import { act, renderHook, waitFor, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useInventoryPagination } from "./use-inventory-pagination";
afterEach(cleanup);
it("restores page numbering and predecessor reads from a copied page-three URL", async () => {
  const update = vi.fn(),
    fetchPage = vi.fn(async (cursor: string | undefined) => ({
      next_cursor: cursor ? "c" + (Number(cursor.slice(1)) + 1) : "c2",
    }));
  const { result, rerender } = renderHook(
    ({ location }) =>
      useInventoryPagination({
        scopeKey: "actor/type/ns/filter/v1",
        location,
        update,
        fetchPage,
      }),
    { initialProps: { location: { page_cursor: "c3", page_index: 3 } } },
  );
  expect(result.current.page).toBe(3);
  act(() => result.current.go(2));
  await waitFor(() =>
    expect(update).toHaveBeenCalledWith({
      page_cursor: "c2",
      page_index: 2,
      resource_uid: undefined,
    }),
  );
  expect(fetchPage).toHaveBeenCalledTimes(1);
  rerender({ location: { page_cursor: "c2", page_index: 2 } });
  act(() => result.current.go(1));
  expect(update).toHaveBeenLastCalledWith({
    page_cursor: undefined,
    page_index: undefined,
    resource_uid: undefined,
  });
});
it("limits recovery to eight reads per explicit batch", async () => {
  const update = vi.fn(),
    fetchPage = vi.fn(async (cursor: string | undefined) => ({
      next_cursor: "c" + (cursor ? Number(cursor.slice(1)) + 1 : 2),
    }));
  const { result } = renderHook(() =>
    useInventoryPagination({
      scopeKey: "scope",
      location: { page_cursor: "c21", page_index: 21 },
      update,
      fetchPage,
    }),
  );
  act(() => result.current.go(20));
  await waitFor(() => expect(result.current.recovery?.reached).toBe(9));
  expect(fetchPage).toHaveBeenCalledTimes(8);
  expect(update).not.toHaveBeenCalled();
  act(() => result.current.continueRecovery());
  await waitFor(() => expect(result.current.recovery?.reached).toBe(17));
  expect(fetchPage).toHaveBeenCalledTimes(16);
  act(() => result.current.continueRecovery());
  await waitFor(() => expect(update).toHaveBeenCalled());
  expect(fetchPage).toHaveBeenCalledTimes(19);
});
it("cancels restoration when the authorized scope changes", async () => {
  let finish!: (value: { next_cursor: string }) => void;
  let signal: AbortSignal | undefined;
  const update = vi.fn();
  const { result, rerender } = renderHook(
    ({ scope }) =>
      useInventoryPagination({
        scopeKey: scope,
        location: { page_cursor: "c3", page_index: 3 },
        update,
        fetchPage: (_cursor, cancel) => {
          signal = cancel;
          return new Promise((resolve) => {
            finish = resolve;
          });
        },
      }),
    { initialProps: { scope: "one" } },
  );
  act(() => result.current.go(2));
  await waitFor(() => expect(finish).toBeDefined());
  rerender({ scope: "two" });
  expect(signal?.aborted).toBe(true);
  await act(async () => finish({ next_cursor: "foreign" }));
  expect(update).not.toHaveBeenCalled();
});
