// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { fixtures, presentTool } from "./test-support";

afterEach(() => document.body.replaceChildren());
for (const locale of ["zh-CN", "en-US"]) {
  const zh = locale === "zh-CN";
  for (const health of [
    "normal",
    "error",
    "no_data",
    "authorization_denied",
    "unknown",
    "recovered",
  ] as const) {
    it(`distinguishes alert health, notification and retained values ${health} / ${locale}`, async () => {
      const sample = fixtures.scenarios[`health-${health}`]!;
      presentTool(sample.Tool, sample.Data, locale);
      await vi.waitFor(() =>
        expect(document.querySelector("[data-alert-health]")).not.toBeNull(),
      );
      const status = document.querySelector("[data-alert-health]")!;
      const labels: Record<string, string[]> = {
        normal: ["正常", "Normal"],
        recovered: ["正常", "Normal"],
        error: ["评估失败", "Evaluation error"],
        no_data: ["无数据", "No data"],
        authorization_denied: ["评估权限不足", "Evaluation access denied"],
        unknown: ["未知", "Unknown"],
      };
      const field = (label: string) =>
        [...status.querySelectorAll("dt")].find(
          (node) => node.textContent === label,
        )?.nextElementSibling?.textContent;
      expect(field(zh ? "评估健康" : "Evaluation health")).toBe(
        labels[health]![zh ? 0 : 1],
      );
      expect(field(zh ? "生命周期" : "Lifecycle")).toBe(zh ? "活跃" : "Firing");
      expect(field(zh ? "通知状态" : "Notification status")).toBe(
        zh ? "失败" : "Failed",
      );
      const retained = !["normal", "recovered"].includes(health);
      expect(
        status.textContent?.includes(
          zh ? "保留上次成功评估值" : "Retained last successful values",
        ),
      ).toBe(retained);
      expect(
        document.querySelector("[data-alert-members]")?.textContent,
      ).toContain("4,096.75 MiB");
      const successful = field(
        zh ? "最近成功评估" : "Last successful evaluation",
      );
      const attempted = field(zh ? "最近评估尝试" : "Last evaluation attempt");
      if (retained) expect(successful).not.toBe(attempted);
      else expect(successful).toBe(attempted);
      if (health === "unknown")
        expect(successful).toBe(zh ? "未知" : "Unknown");
    });
  }
  for (const view of ["events", "history"] as const) {
    it(`exposes unhealthy retained values before and after expansion ${view} / ${locale}`, async () => {
      const sample = fixtures.scenarios[`health-error-${view}`]!;
      presentTool(sample.Tool, sample.Data, locale);
      await vi.waitFor(() =>
        expect(document.querySelector("tbody")?.textContent).toContain(
          zh ? "评估失败" : "Evaluation error",
        ),
      );
      expect(document.querySelector("tbody")?.textContent).toContain(
        zh ? "保留" : "Retained",
      );
      (document.querySelector("tbody button") as HTMLButtonElement).click();
      expect(
        document.querySelector("[data-alert-health]")?.textContent,
      ).toContain(zh ? "评估失败" : "Evaluation error");
    });
  }
  for (const version of ["known", "unknown", "empty"] as const) {
    it(`uses the connector software version in the main column ${version} / ${locale}`, async () => {
      const sample = fixtures.scenarios[`version-${version}`]!;
      presentTool(sample.Tool, sample.Data, locale);
      if (version === "empty") {
        await vi.waitFor(() =>
          expect(document.body.textContent).toContain(
            zh ? "当前范围没有数据" : "No data in this range",
          ),
        );
        return;
      }
      await vi.waitFor(() =>
        expect(document.querySelector("tbody tr")).not.toBeNull(),
      );
      const headers = [...document.querySelectorAll("thead th")].map(
        (node) => node.textContent,
      );
      const index = headers.indexOf(zh ? "软件版本" : "Software version");
      expect(index).toBeGreaterThan(-1);
      expect(
        document.querySelector("tbody tr")!.children[index]?.textContent,
      ).toBe(version === "known" ? "v9.8.7" : zh ? "未知" : "Unknown");
    });
  }
}
