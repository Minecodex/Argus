// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryBoundary } from "./query-boundary";
import { LocaleProvider } from "./locale";
afterEach(cleanup);
it("distinguishes loading, denied and failure from actual empty data, and retries", () => {
  localStorage.setItem("argus.locale", "en-US");
  const refetch = vi.fn(),
    query = { isLoading: true, error: null as unknown, refetch };
  const view = (q: typeof query) => (
    <LocaleProvider>
      <QueryBoundary query={q}>
        <div>No configured objects</div>
      </QueryBoundary>
    </LocaleProvider>
  );
  const { rerender } = render(view(query));
  expect(screen.getByRole("status")).toHaveTextContent("Loading");
  expect(screen.queryByText("No configured objects")).toBeNull();
  rerender(view({ ...query, isLoading: false, error: { status: 403 } }));
  expect(screen.getByText("Access denied")).toBeVisible();
  expect(screen.queryByText("No configured objects")).toBeNull();
  rerender(view({ ...query, isLoading: false, error: new Error("offline") }));
  expect(screen.getByText("Failed to load data")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(refetch).toHaveBeenCalledOnce();
  rerender(view({ ...query, isLoading: false }));
  expect(screen.getByText("No configured objects")).toBeVisible();
});

it("does not manufacture healthy summaries when a dependency fails, prioritizes denial, and retries the whole dependency set", async () => {
  localStorage.setItem("argus.locale", "en-US");
  const first = {
    isPending: false,
    error: new Error("offline"),
    refetch: vi.fn(() => Promise.reject(new Error("still offline"))),
  };
  const second = { isPending: true, error: { status: 403 }, refetch: vi.fn() };
  render(
    <LocaleProvider>
      <QueryBoundary query={first} dependencies={[second]}>
        <div>0 members</div>
      </QueryBoundary>
    </LocaleProvider>,
  );
  expect(screen.getByText("Access denied")).toBeVisible();
  expect(screen.queryByText("0 members")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(first.refetch).toHaveBeenCalledOnce();
  expect(second.refetch).toHaveBeenCalledOnce();
  await Promise.resolve();
});

it("distinguishes a missing object from an indefinitely loading editor", () => {
  localStorage.setItem("argus.locale", "en-US");
  render(
    <LocaleProvider>
      <QueryBoundary
        query={{ error: { status: 404 }, refetch: vi.fn(), isPending: false }}
      >
        <div>Editor</div>
      </QueryBoundary>
    </LocaleProvider>,
  );
  expect(screen.getByText("Content not found")).toBeVisible();
  expect(screen.queryByText("Editor")).toBeNull();
});
