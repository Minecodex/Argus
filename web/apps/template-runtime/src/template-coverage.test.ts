// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { fixtures, presentTool } from "./test-support";
afterEach(() => document.body.replaceChildren());
it.each(["zh-CN", "en-US"])(
  "uses zero pagination for an empty list in %s",
  async (locale) => {
    presentTool("host.list", { items: [] }, locale);
    await vi.waitFor(() =>
      expect(
        document.querySelector(".argus-template-pagination")?.textContent,
      ).toContain("0"),
    );
    expect(document.body.textContent).toContain(
      locale === "zh-CN" ? "当前范围没有数据" : "No data in this range",
    );
  },
);
it("covers every production tool binding and exact source asset", () => {
  expect(fixtures.bindings).toHaveLength(106);
  for (const binding of fixtures.bindings) {
    expect(["template", "trace", "file", "action"]).toContain(binding.Mode);
    if (binding.Mode !== "template") continue;
    expect(binding.Hash).toMatch(/^[a-f0-9]{64}$/);
    expect(binding.Version).toMatch(/\/v2$/);
    expect(binding.Reads).toContain("result.detail");
    expect(() => presentTool(binding.Tool)).not.toThrow();
    expect(document.body.textContent!.length).toBeGreaterThan(10);
  }
});
for (const binding of fixtures.bindings.filter(
  (row) => row.Mode === "template",
))
  it(`shows the declared business content for ${binding.Tool}`, async () => {
    presentTool(binding.Tool, undefined, "en-US");
    await vi.waitFor(() => {
      for (const expected of binding.Expected)
        expect(document.body.textContent).toContain(expected);
    });
  });
const entries = new Map(
  fixtures.bindings
    .filter((row) => row.Mode === "template")
    .map((row) => [row.Entry, row.Tool]),
);
for (const locale of ["zh-CN", "en-US"])
  for (const [entry, tool] of entries) {
    const cases = fixtures.cases[entry as keyof typeof fixtures.cases];
    for (const [variant, data] of Object.entries(cases)) {
      it(
        "renders escaped " + entry + " / " + variant + " / " + locale,
        async () => {
          expect(() => presentTool(tool, data, locale)).not.toThrow();
          expect(
            document.querySelector("script,img,iframe,form,object"),
          ).toBeNull();
          await vi.waitFor(() => {
            if (variant === "long-name")
              expect(document.body.textContent).toContain("Boundary long name");
            if (variant === "special-characters")
              expect(document.body.textContent).toContain(
                "<script>alert('xss')</script>",
              );
          });
        },
      );
    }
  }
