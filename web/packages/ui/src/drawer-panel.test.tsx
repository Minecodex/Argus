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
import { DrawerPanel } from "./drawer-panel";
import { FormDrawer } from "./overlays";
import { Field, Input } from "./form";
import { LocaleProvider } from "./locale";
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
it("submits the drawer form from the separate shared footer", async () => {
  const submit = vi.fn(),
    close = vi.fn();
  render(
    <LocaleProvider>
      <FormDrawer
        open
        title="Edit resource"
        onOpenChange={close}
        onSubmit={submit}
        submitLabel="Save"
      >
        <Field label="Name" requirement="required">
          <Input defaultValue="Resource A" />
        </Field>
      </FormDrawer>
    </LocaleProvider>,
  );
  await screen.findByRole("dialog", { name: "Edit resource" });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(close).not.toHaveBeenCalled();
});
it("offers a read-only drawer without a submit form or action", async () => {
  render(
    <LocaleProvider>
      <DrawerPanel open title="Linked resources" onOpenChange={() => {}}>
        <p>Resource A</p>
      </DrawerPanel>
    </LocaleProvider>,
  );
  const dialog = await screen.findByRole("dialog", {
    name: "Linked resources",
  });
  expect(dialog.querySelector("form")).toBeNull();
  expect(dialog.querySelector('button[type="submit"]')).toBeNull();
  expect(screen.getByRole("button", { name: "Close" })).toBeVisible();
});
