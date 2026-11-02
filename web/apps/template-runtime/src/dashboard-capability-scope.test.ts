// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { presentTool } from "./test-support";

afterEach(() => document.body.replaceChildren());

it.each(["zh-CN", "en-US"])("keeps capability links per resource in %s", async (locale) => {
  presentTool("observe.capabilities.get", {
    complete: true, sampling_performed: false, data_state: "not_checked",
    items: [{ name: "Host", sections: { dashboards: { status: "available", complete: true, items: [] } } }],
  }, locale);
  await vi.waitFor(() => expect(document.body.textContent).toContain("Host"));
  const expand = [...document.querySelectorAll("button")].find(button => button.textContent === (locale === "zh-CN" ? "展开" : "Expand"));
  expect(expand).toBeDefined();
  expand!.click();
  await vi.waitFor(() => expect(document.body.textContent).toContain(locale === "zh-CN" ? "不是多个资源组合的适用性结论" : "not applicability for a combined resource set"));
});
