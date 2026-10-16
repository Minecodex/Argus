// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ToolResultDisclosure } from "./disclosure";
Element.prototype.getAnimations ??= () => [];

beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("keeps the selected result alive while switching views, then releases it on collapse", () => {
  const mount = vi.fn(),
    destroy = vi.fn();
  function View() {
    useEffect(() => {
      mount();
      return destroy;
    }, []);
    return <input aria-label="Result filter" defaultValue="" />;
  }
  render(
    <ToolResultDisclosure
      title="Query"
      viewLabel="Open"
      collapseLabel="Close"
      views={[
        { key: "view", label: "View", content: <View /> },
        {
          key: "output",
          label: "Returned content",
          content: <pre>Stored output</pre>,
        },
        {
          key: "input",
          label: "Query parameters",
          content: <pre>Stored parameters</pre>,
        },
      ]}
    />,
  );
  expect(mount).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Open · Query" }));
  const filter = screen.getByLabelText("Result filter");
  fireEvent.change(filter, { target: { value: "orders" } });
  fireEvent.click(screen.getByRole("tab", { name: "Returned content" }));
  expect(screen.getByText("Stored output")).toBeVisible();
  expect(filter).not.toBeVisible();
  fireEvent.click(screen.getByRole("tab", { name: "Query parameters" }));
  expect(screen.getByText("Stored parameters")).toBeVisible();
  fireEvent.click(screen.getByRole("tab", { name: "View" }));
  expect(filter).toBeVisible();
  expect(filter).toHaveValue("orders");
  expect(mount).toHaveBeenCalledOnce();
  expect(destroy).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(destroy).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("button", { name: "Open · Query" }));
  expect(screen.getByRole("tab", { name: "View" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
});
it("mounts a result only after viewing and releases it on collapse", () => {
  const mount = vi.fn(),
    destroy = vi.fn();
  function Result() {
    useEffect(() => {
      mount();
      return destroy;
    }, []);
    return <div>Rendered result</div>;
  }
  render(
    <ToolResultDisclosure
      title="Hosts"
      summary="Production · 12 returned"
      status="success"
      statusLabel="Completed"
      viewLabel="View"
      collapseLabel="Collapse"
    >
      <Result />
    </ToolResultDisclosure>,
  );
  expect(screen.getByText("Production · 12 returned")).toBeVisible();
  expect(mount).not.toHaveBeenCalled();
  const view = screen.getByRole("button", { name: "View · Hosts" });
  expect(view).toHaveAttribute("aria-expanded", "false");
  fireEvent.click(view);
  expect(mount).toHaveBeenCalledOnce();
  expect(screen.getByRole("region", { name: "Hosts" })).toHaveTextContent(
    "Rendered result",
  );
  expect(view).toHaveAttribute("aria-controls", screen.getByRole("region").id);
  fireEvent.click(screen.getByRole("button", { name: "Collapse" }));
  expect(destroy).toHaveBeenCalledOnce();
  expect(screen.queryByRole("region")).toBeNull();
  expect(view).toHaveFocus();
  fireEvent.click(view);
  expect(mount).toHaveBeenCalledTimes(2);
});

it("shows a failure summary even when the body is closed", () => {
  render(
    <ToolResultDisclosure
      title="Query"
      summary="Query timed out"
      status="failed"
      statusLabel="Failed"
      viewLabel="View"
      collapseLabel="Collapse"
    >
      <div>Detail</div>
    </ToolResultDisclosure>,
  );
  expect(screen.getByText("Query timed out")).toBeVisible();
  expect(screen.getByRole("img", { name: "Failed" })).toBeVisible();
  expect(screen.queryByText("Detail")).toBeNull();
});
