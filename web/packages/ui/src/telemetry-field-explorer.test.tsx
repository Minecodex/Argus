// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  TelemetryFieldExplorer,
  type TelemetryCatalogPage,
} from "./telemetry-field-explorer";
import { LocaleProvider } from "./locale";
afterEach(cleanup);
it("freezes pagination scope, distinguishes failures from absence and aborts removed explorers", async () => {
  localStorage.setItem("argus.locale", "en-US");
  const used = vi.fn();
  const page = (fields: string[], next?: string): TelemetryCatalogPage => ({
    fields: fields.map((name) => ({ name, type: "string" })),
    values: [],
    has_more: !!next,
    next_cursor: next,
  });
  const load = vi
    .fn<
      (
        request: Parameters<
          React.ComponentProps<typeof TelemetryFieldExplorer>["load"]
        >[0],
        signal: AbortSignal,
      ) => Promise<TelemetryCatalogPage>
    >()
    .mockResolvedValueOnce(page(["service_name"], "fields-next"))
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce({ ...page([]), values: ["orders"] })
    .mockResolvedValueOnce(page(["trace_id"]))
    .mockResolvedValueOnce(page(["body"], "refreshed-next"))
    .mockImplementationOnce(() => new Promise(() => {}));
  const view = render(
    <LocaleProvider>
      <TelemetryFieldExplorer scope="Source A" load={load} onUse={used} />
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Load fields" }));
  fireEvent.click(await screen.findByRole("button", { name: "service_name" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "absence has not been confirmed",
  );
  expect(
    screen.queryByText("No matching non-empty values"),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Load field values" }));
  fireEvent.click(
    await screen.findByRole("button", {
      name: "Add filter service_name = orders",
    }),
  );
  expect(used).toHaveBeenCalledWith("service_name", "orders");
  fireEvent.click(screen.getByRole("button", { name: "More fields" }));
  await screen.findByRole("button", { name: "trace_id" });
  expect(load.mock.calls[3]![0]).toMatchObject({
    kind: "fields",
    cursor: "fields-next",
    field: undefined,
    search: "",
  });
  fireEvent.click(screen.getByRole("button", { name: "Load fields" }));
  await screen.findByRole("button", { name: "body" });
  expect(load.mock.calls[4]![0].field).toBeUndefined();
  fireEvent.change(screen.getByRole("textbox", { name: "Search fields" }), {
    target: { value: "a" },
  });
  expect(
    screen.queryByRole("button", { name: "More fields" }),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Load fields" }));
  await waitFor(() => expect(load).toHaveBeenCalledTimes(6));
  const signal = load.mock.calls[5]![1];
  view.unmount();
  expect(signal.aborted).toBe(true);
});
