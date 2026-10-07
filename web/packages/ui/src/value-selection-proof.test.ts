import { expect, it } from "vitest";
import { selectionDisappeared } from "./value-selection-proof";
it("preserves selections outside a page, a search or an incomplete result", () => {
  expect(
    selectionDisappeared(["saved"], {
      values: ["first"],
      complete: false,
      next_cursor: "page2",
    }),
  ).toBe(false);
  expect(selectionDisappeared(["saved"], { values: [], complete: false })).toBe(
    false,
  );
  expect(
    selectionDisappeared(["saved"], {
      values: [],
      complete: true,
      next_cursor: "page2",
    }),
  ).toBe(false);
});
it("resets only on exact absence evidence or a complete result", () => {
  expect(
    selectionDisappeared(["saved"], {
      values: [],
      complete: false,
      selected_exists: { saved: true },
    }),
  ).toBe(false);
  expect(
    selectionDisappeared(["saved"], {
      values: [],
      complete: false,
      selected_exists: { saved: false },
    }),
  ).toBe(true);
  expect(selectionDisappeared(["saved"], { values: [], complete: true })).toBe(
    true,
  );
});
