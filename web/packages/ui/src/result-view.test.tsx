// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ResultTable, RecordFields } from "./result-view";
import { LocaleProvider } from "./locale";
afterEach(cleanup);
it("opens the exact selected row from its resource name without interpreting markup", () => {
  render(
    <LocaleProvider locale="en-US" persist={false}>
      <ResultTable
        rows={[{ id: "a", name: "<script>name</script>" }]}
        columns={[{ label: "Name", value: (row) => String(row.name) }]}
        detail={(target, row) => {
          target.textContent = `Detail ${row.id}`;
        }}
      />
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("link", { name: "<script>name</script>" }));
  expect(screen.getByText("Detail a")).toBeVisible();
  expect(document.querySelector("script")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Collapse" }));
  expect(screen.queryByText("Detail a")).toBeNull();
});
it("keeps short factual arrays visible, including alert suppression reasons", () => {
  render(
    <RecordFields fields={[["Reasons", ["maintenance", "upstream outage"]]]} />,
  );
  expect(screen.getByText("maintenance · upstream outage")).toBeVisible();
});
