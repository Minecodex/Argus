// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Pagination } from "./pagination";
afterEach(cleanup);

it("renders bounded page numbers, ellipses and an accessible current page", () => {
  const change = vi.fn();
  render(<Pagination page={6} totalPages={200} onChange={change} />);
  expect(screen.getByRole("button", { name: "第 6 页" })).toHaveAttribute(
    "aria-current",
    "page",
  );
  expect(screen.getAllByRole("button").length).toBeLessThanOrEqual(9);
  fireEvent.click(screen.getByRole("button", { name: "第 200 页" }));
  expect(change).toHaveBeenCalledWith(200);
});
it("never offers unknown cursor pages and blocks navigation while fetching", () => {
  const change = vi.fn();
  const { rerender } = render(
    <Pagination page={3} hasNext onChange={change} />,
  );
  expect(
    screen.queryByRole("button", { name: "第 5 页" }),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "第 1 页" }));
  expect(change).toHaveBeenCalledWith(1);
  rerender(<Pagination page={3} hasNext isDisabled onChange={change} />);
  for (const button of screen.getAllByRole("button"))
    expect(button).toBeDisabled();
});
it("keeps single-page boundaries disabled and does not fabricate a zero page", () => {
  render(<Pagination page={1} totalPages={0} onChange={vi.fn()} />);
  expect(screen.getByRole("button", { name: "上一页" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "下一页" })).toBeDisabled();
  expect(screen.getByText("第 1 / 1 页")).toBeInTheDocument();
});
