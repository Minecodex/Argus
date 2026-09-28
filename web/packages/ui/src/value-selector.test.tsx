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
import { LocaleProvider } from "./locale";
import { ValueSelector } from "./value-selector";
afterEach(cleanup);
it("labels source identities without submitting their presentation labels", async () => {
  localStorage.setItem("argus.locale", "en-US");
  const change = vi.fn();
  render(
    <LocaleProvider>
      <ValueSelector
        label="Source"
        value={{ all: false, values: ["old-id"] }}
        candidates={["old-id", "new-id"]}
        valueLabels={{
          "old-id": "Historical installation",
          "new-id": "Current installation",
        }}
        onChange={change}
      />
    </LocaleProvider>,
  );
  expect(
    screen.getByRole("button", { name: "Source" }),
  ).toHaveAccessibleDescription("Source: Historical installation");
  fireEvent.click(screen.getByRole("button", { name: "Source" }));
  fireEvent.click(screen.getByRole("radio", { name: "Current installation" }));
  fireEvent.click(screen.getByRole("button", { name: "Apply" }));
  expect(change).toHaveBeenCalledWith({ all: false, values: ["new-id"] });
});
it.each([false, true])(
  "does not reset selected values after an incomplete page or a failed lookup (%s)",
  async (failure) => {
    localStorage.setItem("argus.locale", "en-US");
    const change = vi.fn();
    const fetchValues = vi.fn(async () => {
      if (failure) throw new Error("unavailable");
      return { values: ["another"], next_cursor: "next" };
    });
    render(
      <LocaleProvider>
        <ValueSelector
          label="Environment"
          value={{ all: false, values: ["production"] }}
          candidates={[]}
          fetchValues={fetchValues}
          onChange={change}
        />
      </LocaleProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Environment" }));
    await waitFor(() => expect(fetchValues).toHaveBeenCalled());
    if (failure)
      expect(await screen.findByRole("alert")).toHaveTextContent("unchanged");
    else await screen.findByRole("radio", { name: "another" });
    expect(screen.getByRole("radio", { name: "production" })).toBeChecked();
    expect(change).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(change).toHaveBeenCalledWith({ all: false, values: ["production"] });
  },
);

it("reloads an open selector after scope changes and ignores the old page", async () => {
  localStorage.setItem("argus.locale", "en-US");
  const change = vi.fn();
  let oldResolve!: (page: { values: string[] }) => void;
  let oldSignal!: AbortSignal;
  const oldFetch = vi.fn(
    (_search: string, _cursor: string | undefined, signal: AbortSignal) => {
      oldSignal = signal;
      return new Promise<{ values: string[] }>((resolve) => {
        oldResolve = resolve;
      });
    },
  );
  const props = {
    label: "Environment",
    value: { all: false, values: ["production"] },
    candidates: [],
    onChange: change,
  };
  const view = render(
    <LocaleProvider>
      <ValueSelector {...props} contextKey="first" fetchValues={oldFetch} />
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Environment" }));
  await waitFor(() => expect(oldFetch).toHaveBeenCalledTimes(1));
  view.rerender(
    <LocaleProvider>
      <ValueSelector
        {...props}
        contextKey="second"
        fetchValues={async () => ({ values: ["new-scope"] })}
      />
    </LocaleProvider>,
  );
  expect(oldSignal.aborted).toBe(true);
  await screen.findByRole("radio", { name: "new-scope" });
  oldResolve({ values: ["stale-scope"] });
  await waitFor(() =>
    expect(screen.queryByRole("radio", { name: "stale-scope" })).toBeNull(),
  );
  expect(screen.getByRole("radio", { name: "production" })).toBeChecked();
  expect(change).not.toHaveBeenCalled();
});

it("uses the reconciled selection after an upstream change while the selector is open", async () => {
  localStorage.setItem("argus.locale", "en-US");
  const change = vi.fn();
  const props = {
    label: "Member",
    candidates: [],
    onChange: change,
    fetchValues: async () => ({ values: ["new-member"] }),
  };
  const view = render(
    <LocaleProvider>
      <ValueSelector
        {...props}
        contextKey="blue"
        value={{ all: false, values: ["old-member"] }}
      />
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Member" }));
  view.rerender(
    <LocaleProvider>
      <ValueSelector
        {...props}
        contextKey="green"
        value={{ all: true, values: [] }}
        disabled
      />
    </LocaleProvider>,
  );
  expect(screen.getByRole("button", { name: "Apply" })).toBeDisabled();
  view.rerender(
    <LocaleProvider>
      <ValueSelector
        {...props}
        contextKey="green"
        value={{ all: true, values: [] }}
      />
    </LocaleProvider>,
  );
  await screen.findByRole("radio", { name: "new-member" });
  expect(screen.queryByRole("radio", { name: "old-member" })).toBeNull();
  expect(screen.getByRole("checkbox", { name: "All" })).toBeChecked();
  fireEvent.click(screen.getByRole("button", { name: "Apply" }));
  expect(change).toHaveBeenCalledWith({ all: true, values: [] });
});
