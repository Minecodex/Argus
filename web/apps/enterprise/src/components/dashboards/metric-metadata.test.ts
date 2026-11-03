import { expect, it } from "vitest";
import { catalogMetricMetadata, mergeMetricCatalog, metricCatalogOptions } from "./metric-metadata";

const a = { name: "same_name", type: "gauge", unit: "1", labels: ["state"], source_type: "hostmetrics", source_keys: ["one:1"] };
const b = { ...a, type: "counter", unit: "By", source_keys: ["two:1"] };

it("preserves source/type/unit variants across pages without duplicate name options", () => {
  const rows = mergeMetricCatalog([a], [b, a]);
  expect(rows).toHaveLength(2);
  const options = metricCatalogOptions(rows);
  expect(options).toHaveLength(1);
  expect(options[0]?.description).toContain("gauge · 1");
  expect(options[0]?.description).toContain("counter · By");
  expect(catalogMetricMetadata(rows, "same_name", true)).toMatchObject({ type: "mixed", unit: "" });
});

it("does not certify type/unit or a ratio from an incomplete variant page", () => {
  expect(catalogMetricMetadata([a], a.name, false)).toMatchObject({ type: "mixed", unit: "" });
  expect(catalogMetricMetadata([a], a.name, true)).toBe(a);
  expect(catalogMetricMetadata([a], "unknown", true)).toBeUndefined();
});
