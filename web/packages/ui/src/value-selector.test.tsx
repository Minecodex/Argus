// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ValueSelector } from "./value-selector";
import { LocaleProvider } from "./locale";
import { Field } from "./form";
beforeEach(() => {
  localStorage.setItem("argus.locale", "en-US");
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
      unobserve() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("keeps popover fields separate from the trigger field and cancels without applying", async () => {
  const change = vi.fn();
  render(
    <LocaleProvider>
      <Field label="Parent resource" requirement="required">
        <ValueSelector
          presentation="popover"
          label="Environment"
          value={{ all: true, values: [] }}
          candidates={["production"]}
          onChange={change}
        />
      </Field>
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Environment" }));
  await screen.findByRole("dialog", { name: "Environment" });
  const search = screen.getByRole("textbox", {
    name: "Search candidates",
  });
  expect(search).not.toHaveAttribute("aria-labelledby");
  expect(search).not.toHaveAttribute("required");
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  await waitFor(() =>
    expect(
      screen.queryByRole("dialog", { name: "Environment" }),
    ).not.toBeInTheDocument(),
  );
  expect(change).not.toHaveBeenCalled();
});

it("adds directory candidates that arrive after opening without clearing selection", async () => {
  const change = vi.fn();
  const props = {
    presentation: "popover" as const,
    label: "Resources",
    multiple: true,
    value: { all: false, values: ["first"] },
    onChange: change,
  };
  const view = render(
    <LocaleProvider>
      <ValueSelector {...props} candidates={[]} />
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Resources" }));
  await screen.findByRole("checkbox", { name: "first" });
  view.rerender(
    <LocaleProvider>
      <ValueSelector {...props} candidates={["first", "second"]} />
    </LocaleProvider>,
  );
  expect(
    await screen.findByRole("checkbox", { name: "second" }),
  ).not.toBeChecked();
  expect(screen.getByRole("checkbox", { name: "first" })).toBeChecked();
  expect(change).not.toHaveBeenCalled();
});

it("preserves saved values on paging/failure and resets on exact disappearance", async () => {
  const change = vi.fn(),
    fetchValues = vi
      .fn()
      .mockResolvedValueOnce({
        values: ["first"],
        next_cursor: "page2",
        complete: false,
      })
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({
        values: [],
        complete: false,
        selected_exists: { saved: false },
      });
  render(
    <LocaleProvider>
      <ValueSelector
        label="Service"
        value={{ all: false, values: ["saved"] }}
        candidates={[]}
        onChange={change}
        fetchValues={fetchValues}
      />
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Service" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "More candidates" }),
    ).toBeEnabled(),
  );
  expect(screen.getByRole("radio", { name: "saved" })).toBeChecked();
  expect(change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "More candidates" }));
  await waitFor(() =>
    expect(screen.getByRole("alert")).toHaveTextContent(
      "selection is unchanged",
    ),
  );
  expect(change).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  await waitFor(() =>
    expect(change).toHaveBeenCalledWith({ all: true, values: [] }),
  );
});
it("aborts an old scope and ignores its late candidate response", async () => {
  let release!: (value: {
    values: string[];
    selected_exists: Record<string, boolean>;
  }) => void;
  const delayed = new Promise<{
    values: string[];
    selected_exists: Record<string, boolean>;
  }>((resolve) => {
    release = resolve;
  });
  const change = vi.fn(),
    fetchValues = vi
      .fn()
      .mockReturnValueOnce(delayed)
      .mockResolvedValue({
        values: ["saved"],
        selected_exists: { saved: true },
      });
  const props = {
    label: "Service",
    value: { all: false, values: ["saved"] },
    candidates: [],
    onChange: change,
    fetchValues,
  };
  const { rerender } = render(
    <LocaleProvider>
      <ValueSelector {...props} contextKey="old" />
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Service" }));
  await waitFor(() => expect(fetchValues).toHaveBeenCalledTimes(1));
  const signal = fetchValues.mock.calls[0]![2] as AbortSignal;
  rerender(
    <LocaleProvider>
      <ValueSelector {...props} contextKey="new" />
    </LocaleProvider>,
  );
  expect(signal.aborted).toBe(true);
  await waitFor(() => expect(fetchValues).toHaveBeenCalledTimes(2));
  release({ values: ["stale"], selected_exists: { saved: false } });
  await waitFor(() =>
    expect(screen.getByRole("radio", { name: "saved" })).toBeChecked(),
  );
  expect(
    screen.queryByRole("radio", { name: "stale" }),
  ).not.toBeInTheDocument();
  expect(change).not.toHaveBeenCalled();
});
