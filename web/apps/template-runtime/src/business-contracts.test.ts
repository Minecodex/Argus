// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { fixtures, presentTool } from "./test-support";
afterEach(() => document.body.replaceChildren());
for (const locale of ["zh-CN", "en-US"])
  for (const [name, scenario] of Object.entries(fixtures.scenarios).filter(([, row]) => row.Mode === "template"))
    it(`renders business contract ${name} in ${locale}`, async () => {
      presentTool(scenario.Tool, scenario.Data, locale);
      await vi.waitFor(() => {
        for (const expected of scenario.Expected)
          expect(document.body.textContent).toContain(expected);
      });
      if (name === "version-empty" || name === "log-aggregate-empty") {
        expect(document.body.textContent).toContain(
          locale === "zh-CN" ? "当前范围没有数据" : "No data in this range",
        );
      } else if (name.startsWith("apm-limit-queryAPMTopology-")) {
        // A valid isolated service has one node and no edges. The empty edge
        // collection must not make its non-empty node result look absent.
        await vi.waitFor(() =>
          expect(document.querySelector("tbody")?.textContent).toContain(
            "orders-api",
          ),
        );
        expect(
          document
            .querySelector("[data-trace-result-kind]")
            ?.getAttribute("data-trace-result-kind"),
        ).toBe("apm_topology");
      } else {
        expect(document.body.textContent).not.toContain(
          "No data in this range",
        );
        expect(document.body.textContent).not.toContain("当前范围没有数据");
      }
    });
it("shows real collector revisions and last seen in the summary", async () => {
  presentTool("telemetry.collector.get", undefined, "en-US");
  await vi.waitFor(() => {
    const fields = Object.fromEntries(
      [...document.querySelectorAll("dt")].map((dt) => [
        dt.textContent,
        dt.nextElementSibling?.textContent,
      ]),
    );
    expect(fields["Desired / effective revision"]).toBe("7 / 6");
    expect(fields["Last seen"]).not.toBe("—");
    expect(fields["Convergence"]).toBe("Pending convergence");
    expect(fields["Record version"]).toBe("3");
  });
});
it("shows expressions in the primary rule table and retains the historical threshold", async () => {
  presentTool("alert.policy.get", undefined, "en-US");
  expect(document.querySelector("tbody")?.textContent).toContain(
    "host_cpu_usage_percent",
  );
  expect(document.querySelector("tbody")?.textContent).toContain("> 95 %");
  presentTool("alert.event.get", undefined, "en-US");
  const snapshot = [...document.querySelectorAll("details")].find(
    (node) =>
      node.querySelector("summary")?.textContent ===
      "Policy snapshot at trigger",
  )!;
  expect(snapshot.querySelector("tbody")?.textContent).toContain(
    "host_cpu_usage_percent",
  );
  expect(snapshot.querySelector("tbody")?.textContent).toContain("> 85 %");
  expect(snapshot.querySelector("tbody")?.textContent).not.toContain("> 95 %");
});
