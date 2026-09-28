// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { JsonField } from "./json-field";
import { LocaleProvider } from "./locale";
afterEach(cleanup);
it("retains incomplete JSON but prevents form submission until the visible value is valid", () => {
  localStorage.setItem("argus.locale", "en-US");
  const change = vi.fn();
  const { container } = render(
    <LocaleProvider>
      <form>
        <JsonField
          label="Variables"
          kind="object"
          value={{ region: "a" }}
          onChange={change}
        />
      </form>
    </LocaleProvider>,
  );
  const input = screen.getByRole("textbox", { name: "Variables" });
  fireEvent.change(input, { target: { value: '{"region":' } });
  expect(input).toHaveValue('{"region":');
  expect(container.querySelector("form")!.checkValidity()).toBe(false);
  expect(change).not.toHaveBeenCalled();
  fireEvent.change(input, { target: { value: '{"region":"b"}' } });
  expect(container.querySelector("form")!.checkValidity()).toBe(true);
  expect(change).toHaveBeenCalledWith({ region: "b" });
});
