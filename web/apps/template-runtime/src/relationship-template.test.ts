// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { fixtures, presentTool } from "./test-support";

afterEach(() => document.body.replaceChildren());
const compact = (node: Element | null) =>
  node?.textContent?.replace(/[,\s]/g, "") ?? "";

for (const locale of ["zh-CN", "en-US"])
  for (const mode of ["every", "any"] as const) {
    it(`shows each composite rule observation and frozen combination ${mode} / ${locale}`, async () => {
      const scenario = fixtures.scenarios[`composite-${mode}`];
      presentTool(
        scenario.Tool,
        {
          ...scenario.Data,
          current_configuration: {
            spec: { mode: "independent", rules: [{ threshold: 98765 }] },
          },
        },
        locale,
      );
      await vi.waitFor(() => {
        const members = document.querySelector("[data-alert-members]");
        expect(members?.textContent).toContain("CPU member");
        expect(members?.textContent).toContain("Memory member");
        expect(compact(members)).toContain("97.25%");
        expect(compact(members)).toContain(
          mode === "every" ? "4096.75MiB" : "1024.75MiB",
        );
        expect(compact(members)).toContain(">3072MiB");
        expect(compact(members)).toContain("2048MiB");
        expect(members?.textContent).toContain(
          locale === "zh-CN"
            ? mode === "every"
              ? "全部规则满足"
              : "任一规则满足"
            : mode === "every"
              ? "All rules match"
              : "Any rule matches",
        );
        if (mode === "any")
          expect(members?.textContent).toContain(
            locale === "zh-CN" ? "满足恢复条件" : "Recovery condition met",
          );
      });
      expect(document.body.textContent).not.toContain(
        "00000000-0000-0000-0000-000000000000",
      );
      expect(document.body.textContent).not.toContain("98765");
    });
    for (const view of ["events", "history"] as const)
      it(`expands composite ${view} observations ${mode} / ${locale}`, async () => {
        const scenario = fixtures.scenarios[`composite-${mode}-${view}`];
        presentTool(scenario.Tool, scenario.Data, locale);
        await vi.waitFor(() =>
          expect(document.querySelector("tbody")?.textContent).toContain(
            "Memory member",
          ),
        );
        (document.querySelector("tbody button") as HTMLButtonElement).click();
        expect(
          compact(document.querySelector("[data-alert-members]")),
        ).toContain("97.25%");
        expect(
          document.querySelector("[data-alert-members]")?.textContent,
        ).toContain("host_memory_used_mib");
      });
  }

for (const locale of ["zh-CN", "en-US"])
  for (const kind of [
    "host",
    "cluster",
    "unavailable",
    "unassociated",
  ] as const)
    it(`renders an authorized or unavailable connector relationship ${kind} / ${locale}`, async () => {
      const scenario = fixtures.scenarios[`connector-${kind}`];
      presentTool(scenario.Tool, scenario.Data, locale);
      await vi.waitFor(() =>
        expect(
          document.querySelector("[data-related-resources]"),
        ).not.toBeNull(),
      );
      const related = document.querySelector("[data-related-resources]")!;
      if (kind === "host" || kind === "cluster") {
        const name = kind === "host" ? "db-prod-01" : "orders-production";
        expect(related.textContent).toContain(name);
        expect(related.querySelector("button")?.textContent).toBe(name);
      } else {
        expect(related.querySelector("button")).toBeNull();
        expect(related.textContent).toContain(
          locale === "zh-CN"
            ? kind === "unavailable"
              ? "未授权或已删除"
              : "未关联主机或集群"
            : kind === "unavailable"
              ? "Not authorized or deleted"
              : "No associated host or cluster",
        );
        expect(related.textContent).not.toContain("db-prod-01");
      }
    });
