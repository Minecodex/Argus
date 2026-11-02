// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, createMockApiClient } from "@argus/api-client";
import { afterEach, expect, it, vi } from "vitest";
import { useControlResourceMentions } from "./use-control-resource-mentions";

let permissions = new Set(["connector.read", "telemetry.dashboard.read"]);
vi.mock("../../lib/permissions", () => ({
  useMyPermissions: () => permissions,
}));
afterEach(() => {
  cleanup();
  permissions = new Set(["connector.read", "telemetry.dashboard.read"]);
});
function View() {
  const query = useControlResourceMentions(true);
  return (
    <>
      {query.items.map((item) => (
        <span key={item.id}>{item.label}</span>
      ))}
      {query.isFetching && <p role="status">Checking current authorization</p>}
    </>
  );
}
it("hides old control-plane choices during recheck, on failure and after role permission removal", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  let checking = false,
    finish!: () => void;
  const pending = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const result = async (value: unknown) => {
    if (checking) {
      await pending;
      throw new Error("Resource unavailable");
    }
    return value as never;
  };
  vi.spyOn(api.connectors, "list").mockImplementation(async () =>
    result({ items: [{ id: "connector", name: "Private connector" }] }),
  );
  vi.spyOn(api.dashboards, "list").mockImplementation(async () =>
    result([
      { id: "dashboard", name: "Private dashboard", lifecycle: "active" },
      {
        id: "governance",
        name: "Forbidden model governance",
        lifecycle: "active",
        managed_template_key: "argus.model-governance",
      },
    ]),
  );
  const cache = new QueryClient();
  const tree = () => (
    <ApiProvider client={api}>
      <QueryClientProvider client={cache}>
        <View />
      </QueryClientProvider>
    </ApiProvider>
  );
  const view = render(tree());
  await screen.findByText("Private dashboard");
  await screen.findByText("Private connector");
  expect(
    screen.queryByText("Forbidden model governance"),
  ).not.toBeInTheDocument();
  permissions = new Set();
  view.rerender(tree());
  expect(screen.queryByText("Private dashboard")).not.toBeInTheDocument();
  expect(screen.queryByText("Private connector")).not.toBeInTheDocument();
  permissions = new Set(["connector.read", "telemetry.dashboard.read"]);
  checking = true;
  view.rerender(tree());
  await waitFor(() => expect(cache.isFetching()).toBe(2));
  expect(screen.queryByText("Private dashboard")).not.toBeInTheDocument();
  expect(screen.queryByText("Private connector")).not.toBeInTheDocument();
  expect(screen.getByRole("status")).toBeVisible();
  await act(async () => {
    finish();
  });
  await waitFor(() => expect(cache.isFetching()).toBe(0));
  expect(screen.queryByText("Private dashboard")).not.toBeInTheDocument();
  expect(screen.queryByText("Private connector")).not.toBeInTheDocument();
});
