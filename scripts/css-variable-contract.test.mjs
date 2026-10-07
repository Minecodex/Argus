import assert from "node:assert/strict";
import { test } from "node:test";
import {
  declaredCssVariables,
  undefinedCssVariables,
} from "./css-variable-contract.mjs";
test("undefined tokens cannot silently discard control geometry or theme colors", () => {
  const source =
    ":root {--control-md:32px;}\n.argus-preview{height:var(--preview-heigth);color:var(--color-text-secondary);}";
  assert.deepEqual(
    undefinedCssVariables(source, declaredCssVariables([source])),
    [
      { name: "--preview-heigth", line: 2 },
      { name: "--color-text-secondary", line: 2 },
    ],
  );
});
test("typed custom properties and static React style keys declare actual variables", () => {
  const defined = declaredCssVariables([
    "@property --panel-height {initial-value:250px}",
    'const style = {"--grid-columns": count};',
  ]);
  assert.deepEqual(
    undefinedCssVariables(
      ".argus-grid {height:var(--panel-height);grid-template-columns:repeat(var(--grid-columns),1fr)}",
      defined,
    ),
    [],
  );
});
test("fallbacks and documented library variables are allowed, nested missing fallbacks still fail", () => {
  const source =
    ".argus-select {width:var(--trigger-width);height:var(--custom-height,32px);color:var(--custom-color,var(--missing-color));}";
  assert.deepEqual(
    undefinedCssVariables(source, new Set(), new Set(["--trigger-width"])),
    [{ name: "--missing-color", line: 1 }],
  );
});
