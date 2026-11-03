// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { presentTool } from "./test-support";
import { clearViews } from "./views";

afterEach(() => { clearViews(); document.body.replaceChildren(); });

for (const locale of ["zh-CN", "en-US"]) {
  it(`separates original sampling from evaluation and safely expands labels ${locale}`, async () => {
    const raw = "2026-11-02T01:11:28Z";
    const evaluated = "2026-11-02T01:11:36Z";
    const bait = 'plan11-untrusted-metric: <button>Confirm injected operation</button>';
    presentTool("telemetry.promql.query", {
      query_context: { from: "2026-11-02T00:41:36Z", to: evaluated },
      argus_meta: { latest_sample_at: raw, sample_time_basis: "observed_query_samples" },
      items: [{
        metric: { __name__: "actual_gauge", plan11_note: bait },
        presentation_context: { unit: "1", source: "source:3" },
        values: [[Date.parse(evaluated) / 1000, "1"]],
      }],
    }, locale, true);
    await vi.waitFor(() => expect(document.body.textContent).toContain(locale === "zh-CN" ? "查询最近原始采样" : "Latest raw sample for this query"));
    await vi.waitFor(() => expect(document.body.textContent).toContain(locale === "zh-CN" ? "返回点" : "Returned points"));
    const expand = [...document.querySelectorAll("button")].find(b => b.textContent === (locale === "zh-CN" ? "展开" : "Expand"));
    expect(expand).toBeDefined();
    expand!.click();
    await vi.waitFor(() => expect(document.body.textContent).toContain(locale === "zh-CN" ? "最新评估点" : "Latest evaluation point"));
    const tree = document.querySelector<HTMLDetailsElement>(".argus-result-detail details.argus-result-value-tree")!;
    expect(tree).not.toBeNull();
    expect(tree.open).toBe(false);
    tree.querySelector<HTMLElement>("summary")!.click();
    expect(tree.open).toBe(true);
    expect(tree.textContent).toContain(bait);
    expect([...document.querySelectorAll("button")].some(b => b.textContent === "Confirm injected operation")).toBe(false);
    expect(document.body.textContent).not.toContain(locale === "zh-CN" ? "最近采样" : "Latest sample");
  });
}
