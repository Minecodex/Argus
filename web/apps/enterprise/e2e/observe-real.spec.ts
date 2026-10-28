import { readFileSync } from "node:fs";
import { test, expect, type Locator, type Page } from "@playwright/test";
import { selectTrigger } from "./helpers/select";
import { setChoice } from "./helpers/choice";
const file = process.env.ARGUS_OBSERVE_BROWSER_FIXTURE;
test.skip(
  !file,
  "requires the live owned Collector/Ingest/Kafka/Writer pipeline",
);
const fixture = file
  ? (JSON.parse(readFileSync(file, "utf8")) as {
      api: string;
      session: string;
      csrf: string;
      application: string;
      trace: string;
    })
  : undefined;
const cards = (page: Page) => page.locator(".argus-observe-query");
async function select(
  page: Page,
  root: Locator,
  label: string,
  option: string,
) {
  await selectTrigger(root, label).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}
for (const english of [false, true])
  for (const theme of ["light", "dark"])
    test(`live observation ${english ? "en" : "zh"} ${theme}`, async ({
      page,
      context,
    }, info) => {
      const tx = (zh: string, en: string) => (english ? en : zh);
      await context.addCookies([
        {
          name: "argus_enterprise_session",
          value: fixture!.session,
          domain: "127.0.0.1",
          path: "/",
          httpOnly: true,
          sameSite: "Lax",
        },
        {
          name: "argus_enterprise_csrf",
          value: fixture!.csrf,
          domain: "127.0.0.1",
          path: "/",
          sameSite: "Lax",
        },
      ]);
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      const errors: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.setViewportSize({ width: 1440, height: 1100 });
      await page.goto(
        `/observe?resource_type=application&resource_id=${fixture!.application}&signal=metrics`,
      );
      await expect(
        page.getByRole("heading", {
          name: tx("观测查询", "Observability queries"),
          exact: true,
        }),
      ).toBeVisible();
      const a = cards(page).first();
      await expect(
        a.getByRole("button", {
          name: tx("资源范围 · 已选 1 项", "Resources · 1 selected"),
          exact: true,
        }),
      ).toBeVisible();
      const metric = a.getByRole("combobox", {
        name: english ? /^Metric\*?$/ : /^指标\*?$/,
      });
      await metric.fill("plan8_sdk");
      await metric.press("ArrowDown");
      await page
        .getByRole("option", { name: /plan8_sdk_request_count/ })
        .first()
        .click();
      await expect(metric).toHaveValue("plan8_sdk_request_count");
      await a
        .getByRole("button", {
          name: tx("添加条件", "Add condition"),
          exact: true,
        })
        .click();
      const condition = a.locator(".argus-metric-label-row");
      const label = condition.getByRole("combobox", {
        name: english ? /^Label\*?$/ : /^标签\*?$/,
      });
      await label.fill("argus_resource");
      await label.press("ArrowDown");
      await page
        .getByRole("option", { name: "argus_resource_id", exact: true })
        .click();
      const value = condition.getByRole("combobox", {
        name: tx("值", "Value"),
        exact: true,
      });
      await value.fill(fixture!.application.slice(0, 8));
      await value.press("ArrowDown");
      await page
        .getByRole("option", { name: fixture!.application, exact: true })
        .click();
      await value.press("Tab");
      const run = async (root: Locator) => {
        await root
          .getByRole("button", {
            name: tx("执行查询", "Run query"),
            exact: true,
          })
          .click();
        await expect(
          root
            .getByRole("status")
            .filter({ hasText: tx("查询完成", "Complete") }),
        ).toBeVisible({ timeout: 30000 });
      };
      await run(a);
      await expect(a.getByRole("img")).toBeVisible();
      await expect(a.getByRole("img")).toHaveAttribute("aria-label", /2/);
      await setChoice(
        a.getByRole("radio", {
          name: tx("DSL 编辑器", "DSL editor"),
          exact: true,
        }),
      );
      await expect(a.locator(".cm-editor")).toBeVisible();
      const code = a.getByRole("textbox", {
        name: tx("查询语句", "Query expression"),
        exact: true,
      });
      await code.fill(
        `plan8_sdk_request_count{argus_resource_id="${fixture!.application.slice(0, 8)}`,
      );
      await code.press("Control+Space");
      await expect(
        page
          .getByRole("option", { name: new RegExp(fixture!.application) })
          .first(),
      ).toBeVisible();
      await code.press("Escape");
      await code.fill("sum(plan8_sdk_request_count)");
      await run(a);
      for (const signal of ["logs", "traces"] as const) {
        await a
          .getByRole("button", {
            name: tx("复制查询", "Duplicate query"),
            exact: true,
          })
          .click();
        const card = cards(page).nth(1);
        await select(
          page,
          card,
          tx("查询类型", "Query type"),
          tx(
            signal === "logs" ? "日志" : "链路",
            signal === "logs" ? "Logs" : "Traces",
          ),
        );
        await run(card);
        if (signal === "logs")
          await expect(
            card
              .locator(".argus-observation-log-body")
              .filter({ hasText: "observe-first-event" }),
          ).toBeVisible();
        else
          await expect(
            card.getByText(fixture!.trace, { exact: true }).first(),
          ).toBeVisible();
      }
      await page
        .getByRole("tab", {
          name: tx("指标与字段目录", "Metrics & fields"),
          exact: true,
        })
        .click();
      const catalog = page.locator(".argus-observe-catalog");
      await catalog
        .getByRole("button", { name: english ? /Resources ·/ : /资源范围 ·/ })
        .click();
      const picker = page.getByRole("dialog");
      await picker
        .getByRole("tab", { name: tx("应用", "Applications"), exact: true })
        .click();
      await picker
        .getByRole("button", { name: /Real SDK application/ })
        .click();
      await picker
        .getByRole("button", {
          name: english ? /Confirm selection/ : /确认选择/,
        })
        .click();
      await expect(
        page.getByRole("button", {
          name: "plan8_sdk_request_count",
          exact: true,
        }),
      ).toBeVisible();
      await page
        .getByRole("button", { name: "plan8_sdk_request_count", exact: true })
        .click();
      await expect(page.getByRole("dialog")).toContainText("counter");
      await expect(page.getByRole("dialog")).toContainText("otlp");
      await page.keyboard.press("Escape");
      await page.screenshot({
        path: info.outputPath("live-catalog.png"),
        fullPage: true,
      });
      expect(errors).toEqual([]);
      await page.reload();
      await expect(cards(page)).toHaveCount(1);
      await expect(cards(page).first().getByRole("img")).toHaveCount(0);
    });
