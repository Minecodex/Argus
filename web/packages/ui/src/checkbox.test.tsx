// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { Checkbox } from "./checkbox";

test("square controls expose mixed/checked state and preserve controlled ownership", () => {
  const changed = vi.fn();
  const { rerender } = render(
    <Checkbox
      appearance="outline"
      aria-label="Select rows"
      isSelected={false}
      isIndeterminate
      onChange={changed}
    />,
  );
  const input = screen.getByRole("checkbox", { name: "Select rows" });
  expect(input).toBePartiallyChecked();
  fireEvent.click(input);
  expect(changed).toHaveBeenCalledWith(true);
  expect(input).not.toBeChecked();
  rerender(
    <Checkbox
      appearance="outline"
      aria-label="Select rows"
      isSelected
      isDisabled
      onChange={changed}
    />,
  );
  expect(input).toBeChecked();
  expect(input).not.toBePartiallyChecked();
  expect(input).toBeDisabled();
});
