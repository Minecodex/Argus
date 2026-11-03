// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, createMockApiClient } from "@argus/api-client";
import { afterEach, expect, it, vi } from "vitest";
import { useResourceMentions } from "./use-resource-mentions";

vi.mock("../../lib/permissions", () => ({
  useMyPermissions: () => new Set(["monitoring.read", "application.read"]),
}));
afterEach(cleanup);
function View() {
  const query = useResourceMentions("orders", true);
  return (
    <>
      <div>
        {query.items.map((item) => (
          <span key={`${item.kind}:${item.id}`}>{item.label}</span>
        ))}
      </div>
      <button
        onClick={() => void query.fetchNextPage()}
        disabled={!query.hasNextPage}
      >
        Next test page
      </button>
    </>
  );
}
it("paginates each authorized kind without losing other kinds or leaking unrequested scopes", async () => {
  const api = createMockApiClient({ persist: false, delay: 0 });
  let finishNextPage!: () => void;
  const nextPageReady = new Promise<void>((resolve) => { finishNextPage = resolve; });
  const read = vi
    .spyOn(api.telemetry, "resources")
    .mockImplementation(async (input) => {
      if (input.kind === "application")
        return {
          items: [
            {
              id: "app",
              type: "application",
              name: "orders-api",
              environment: "production",
            },
          ],
        };
      if (input.kind !== "monitoring_target")
        throw new Error("unexpected unauthorized type");
      if (input.cursor) await nextPageReady;
      return input.cursor
        ? {
            items: [
              {
                id: "db2",
                type: "monitoring_target",
                name: "orders-db-2",
                environment: "",
              },
            ],
          }
        : {
            items: [
              {
                id: "db1",
                type: "monitoring_target",
                name: "orders-db-1",
                environment: "",
              },
            ],
            next_cursor: "next-target",
          };
    });
  render(
    <ApiProvider client={api}>
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <View />
      </QueryClientProvider>
    </ApiProvider>,
  );
  await screen.findByText("orders-db-1");
  await screen.findByText("orders-api");
  fireEvent.click(screen.getByRole("button", { name: "Next test page" }));
  await waitFor(() => expect(read).toHaveBeenCalledTimes(3));
  expect(screen.getByText("orders-db-1")).toBeTruthy();
  expect(screen.getByText("orders-api")).toBeTruthy();
  finishNextPage();
  await screen.findByText("orders-db-2");
  expect(screen.getAllByText("orders-api")).toHaveLength(1);
  await waitFor(() => expect(read).toHaveBeenCalledTimes(3));
  expect(read.mock.calls.map(([input]) => input.kind)).toEqual([
    "application",
    "monitoring_target",
    "monitoring_target",
  ]);
  expect(read.mock.calls.every(([input]) => input.search === "orders")).toBe(
    true,
  );
});
