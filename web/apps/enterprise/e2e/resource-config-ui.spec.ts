import { setChoice } from "./helpers/choice";
import { expect, test, type Page, type Locator } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function login(page: Page, english: boolean, theme: string) {
  await page.addInitScript(
    ({ english, theme }) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  await page.goto("/login");
  await page
    .getByRole("textbox", {
      name: english ? "Username" : "用户名",
      exact: true,
    })
    .fill("root");
  await page
    .getByRole("textbox", { name: english ? "Password" : "密码", exact: true })
    .fill("123456");
  await page
    .getByRole("button", { name: english ? "Sign in" : "登录", exact: true })
    .click();
  await expect(page).not.toHaveURL(/login/);
}
async function noOverflow(page: Page) {
  const overflowing = await page
    .locator(
      ".argus-dialog:visible, .argus-drawer:visible, .argus-resource-query:visible",
    )
    .evaluateAll((elements) =>
      elements
        .filter((e) => e.scrollWidth > e.clientWidth + 1)
        .map((e) => e.className),
    );
  expect(overflowing).toEqual([]);
}
async function settled(page: Page) {
  await page.evaluate(async () => {
    await Promise.all(
      document
        .getAnimations()
        .filter((a) => Number.isFinite(a.effect?.getComputedTiming().endTime))
        .map((a) => a.finished.catch(() => undefined)),
    );
  });
}
async function alertNamesOnly(cells: Locator) {
  await expect(cells.first()).toBeVisible();
  for (const cell of await cells.all()) {
    const name = cell.getByRole("button");
    await expect(name).toHaveCount(1);
    await expect(cell).toHaveText(await name.innerText());
    await expect(cell.locator(".argus-alert-tag")).toHaveCount(0);
  }
}
for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    const label = `${english ? "en" : "zh"} ${theme}`;
    test.describe(label, () => {
      test.use({ viewport: { width: english ? 1440 : 1920, height: 1080 } });
      test.setTimeout(60000);
      test.beforeEach(async ({ page }) => login(page, english, theme));
      const tx = (zh: string, en: string) => (english ? en : zh);
      test("current and historical alerts show names while details retain metrics", async ({
        page,
      }, info) => {
        await page.setViewportSize({ width: 1280, height: 800 });
        await page.goto("/alerts");
        const cells = page.locator(".argus-alert-event-cell");
        await alertNamesOnly(cells);
        await cells
          .getByRole("button", {
            name: tx("CPU 使用率持续过高", "Sustained high CPU usage"),
            exact: true,
          })
          .click();
        const details = page.getByRole("dialog");
        await expect(
          details.locator(".argus-alert-code-block").first(),
        ).toContainText("host_cpu_usage_percent");
        await details
          .getByRole("button", { name: tx("关闭", "Close"), exact: true })
          .click();
        await page
          .getByRole("button", {
            name: tx("合并视图", "Grouped view"),
            exact: true,
          })
          .click();
        await alertNamesOnly(cells);
        await page
          .getByRole("button", {
            name: tx("展开列表", "Flat view"),
            exact: true,
          })
          .click();
        await page.screenshot({
          path: info.outputPath("active-alert-names.png"),
          fullPage: true,
          animations: "disabled",
        });
        await page
          .getByRole("tab", {
            name: tx("告警历史", "Alert history"),
            exact: true,
          })
          .click();
        await page
          .getByRole("button", {
            name: new RegExp(tx("全部生命周期", "All lifecycles")),
          })
          .click();
        await page
          .getByRole("option", { name: tx("已恢复", "Resolved"), exact: true })
          .click();
        await expect(
          cells.getByRole("button", {
            name: tx("数据盘容量告警已恢复", "Volume capacity recovered"),
            exact: true,
          }),
        ).toBeVisible();
        await alertNamesOnly(cells);
        await page.screenshot({
          path: info.outputPath("history-alert-names.png"),
          fullPage: true,
          animations: "disabled",
        });
      });
      test("capability cards and visible installation network diagrams", async ({
        page,
      }, info) => {
        await page.goto("/hosts/host-db-master-01#otlp-collector");
        const cards = page.locator(".argus-setting-card");
        await expect(cards).toHaveCount(6);
        for (const card of await cards.all()) {
          await expect(card.locator("p")).not.toBeEmpty();
          await expect(card.getByRole("switch")).toBeVisible();
        }
        const positions = await page
          .locator("#otlp-collector .argus-setting-card")
          .evaluateAll((elements) =>
            elements.map((e) => e.getBoundingClientRect().top),
          );
        expect(positions.length).toBeGreaterThan(1);
        expect(positions[0]).toBeLessThan(positions.at(-1)!);
        await expect(page.locator(".argus-collector-controls")).toHaveCount(0);
        await page.screenshot({
          path: info.outputPath("collection-cards.png"),
          fullPage: true,
          animations: "disabled",
        });
        await page.goto("/hosts");
        for (const name of [
          tx("添加普通主机", "Add Host"),
          tx("添加堡垒机", "Add Bastion"),
        ]) {
          await page.getByRole("button", { name, exact: true }).click();
          const dialog = page.getByRole("dialog", { name, exact: true });
          await expect(dialog).toBeVisible();
          await settled(page);
          const diagrams = dialog.locator(
            ".argus-scenario-card .argus-topology",
          );
          expect(await diagrams.count()).toBeGreaterThanOrEqual(3);
          for (const diagram of await diagrams.all()) {
            const bounds = (await diagram.boundingBox())!;
            expect(bounds.width).toBeGreaterThan(200);
            expect(bounds.height).toBeGreaterThan(70);
          }
          const selection = dialog.locator(
            ".argus-scenario-card:not([disabled])",
          );
          for (const card of await selection.all()) {
            await card.click();
            await expect(card).toHaveAttribute("aria-pressed", "true");
            await expect(
              dialog.locator(".argus-mode-selection__detail .argus-topology"),
            ).toBeVisible();
          }
          await noOverflow(page);
          await page.screenshot({
            path: info.outputPath(
              name === tx("添加普通主机", "Add Host")
                ? "host-network.png"
                : "bastion-network.png",
            ),
            animations: "disabled",
          });
          await dialog
            .getByRole("button", { name: tx("关闭", "Close"), exact: true })
            .click();
        }
      });
      test("temporary resource queries use the dashboard builder without saving a draft", async ({
        page,
      }, info) => {
        const storedDrafts = () =>
          page.evaluate(() => {
            const key = Object.keys(localStorage).find((k) =>
              k.startsWith("argus-mock:db"),
            );
            const db = JSON.parse(localStorage.getItem(key!)!);
            return JSON.stringify({
              drafts: db.dashboards?.drafts ?? [],
              revisions: db.dashboards?.revisions ?? [],
            });
          });
        const before = await storedDrafts();
        for (const url of [
          "/hosts/host-gw-sh-01",
          "/kubernetes/k8s-prod-east",
        ]) {
          await page.goto(url);
          for (const [zh, en] of [
            ["指标", "Metrics"],
            ["日志", "Logs"],
            ["链路", "Traces"],
          ]) {
            await page
              .getByRole("tab", { name: tx(zh!, en!), exact: true })
              .click();
            await expect(
              page
                .getByText(
                  tx(
                    "设置查询条件后执行查询",
                    "Configure a query, then run it",
                  ),
                )
                .first(),
            ).toBeVisible();
            const cards = page.locator(".argus-resource-query > .argus-card");
            await expect(cards).toHaveCount(3);
            await expect(
              cards.nth(1).locator(".argus-card__title").first(),
            ).toHaveText(tx("查询条件", "Query conditions"));
            await expect(
              cards.nth(2).locator(".argus-card__title").first(),
            ).toHaveText(tx(zh!, en!));
            const bounds = await cards.evaluateAll((elements) =>
              elements.map((element) => {
                const { top, bottom } = element.getBoundingClientRect();
                return { top, bottom };
              }),
            );
            expect(bounds[0]!.bottom).toBeLessThanOrEqual(bounds[1]!.top);
            expect(bounds[1]!.bottom).toBeLessThanOrEqual(bounds[2]!.top);
            await page
              .getByRole("button", {
                name: tx("执行查询", "Run query"),
                exact: true,
              })
              .click();
            await expect(
              page.locator(".argus-telemetry-query-meta"),
            ).toBeVisible();
            await setChoice(
              page.getByRole("radio", {
                name: tx("DSL 编辑器", "DSL editor"),
                exact: true,
              }),
            );
            await expect(
              page.getByRole("textbox", {
                name: tx("查询语句", "Query expression"),
                exact: true,
              }),
            ).not.toBeEmpty();
            await page
              .getByRole("button", {
                name: tx("执行查询", "Run query"),
                exact: true,
              })
              .click();
            await expect(
              page.locator(".argus-telemetry-query-meta"),
            ).toBeVisible();
            await noOverflow(page);
          }
        }
        expect(await storedDrafts()).toBe(before);
        await page.screenshot({
          path: info.outputPath("quick-query.png"),
          fullPage: true,
          animations: "disabled",
        });
      });
      test("policy sections, dense fields and safe advanced query switching", async ({
        page,
      }, info) => {
        await page.goto("/alerts");
        const name = tx("创建告警策略", "Create policy");
        await page.getByRole("button", { name, exact: true }).first().click();
        const dialog = page
          .getByRole("dialog")
          .filter({ has: page.getByRole("heading", { name, exact: true }) });
        await expect(
          dialog.locator(".argus-policy-form > .argus-card"),
        ).toHaveCount(4);
        await settled(page);
        await expect(dialog).not.toContainText(/alerts\.\w+/);
        await expect(
          dialog.getByRole("checkbox", {
            name: tx("生产事件群", "Production incidents"),
            exact: true,
          }),
        ).toBeChecked();
        const box = (await dialog.boundingBox())!;
        expect(box.width).toBeGreaterThan(1000);
        const fields = await dialog
          .locator(".argus-policy-rule__conditions .argus-field")
          .evaluateAll((elements) =>
            elements.map((e) => e.getBoundingClientRect().top),
          );
        expect(new Set(fields.map((y) => Math.round(y))).size).toBe(1);
        await setChoice(
          dialog.getByRole("radio", {
            name: tx("高级 PromQL", "Advanced PromQL"),
            exact: true,
          }),
        );
        const expression = dialog.getByRole("textbox", {
          name: tx("PromQL 观测表达式", "PromQL observation expression"),
          exact: true,
        });
        await expression.fill("sum(rate(http_requests_total[5m]))");
        await dialog
          .getByRole("radio", {
            name: tx("简单构建器", "Simple builder"),
            exact: true,
          })
          .press("Space");
        await expect(expression).toHaveValue(
          "sum(rate(http_requests_total[5m]))",
        );
        await expect(
          dialog.getByRole("radio", {
            name: tx("高级 PromQL", "Advanced PromQL"),
            exact: true,
          }),
        ).toBeChecked();
        await noOverflow(page);
        const issues = (
          await new AxeBuilder({ page }).include(".argus-alert-form").analyze()
        ).violations.filter((v) =>
          ["serious", "critical"].includes(v.impact ?? ""),
        );
        expect(issues).toEqual([]);
        await page.screenshot({
          path: info.outputPath("policy-configuration.png"),
          animations: "disabled",
        });
      });
      test("settings drawer fits three related fields in one row", async ({
        page,
      }, info) => {
        await page.goto("/settings/org");
        await page
          .getByRole("tab", { name: tx("用户", "Users"), exact: true })
          .click();
        await page
          .getByRole("button", {
            name: tx("邀请用户", "Invite user"),
            exact: true,
          })
          .click();
        const drawer = page.getByRole("dialog", {
          name: tx("邀请用户", "Invite user"),
          exact: true,
        });
        await expect(drawer).toBeVisible();
        await settled(page);
        expect((await drawer.boundingBox())!.width).toBeGreaterThan(700);
        const fields = drawer.locator(".argus-settings-form > .argus-field");
        const positions = await fields.evaluateAll((elements) =>
          elements
            .slice(0, 3)
            .map((e) => Math.round(e.getBoundingClientRect().top)),
        );
        expect(positions).toHaveLength(3);
        expect(new Set(positions).size).toBe(1);
        await noOverflow(page);
        await page.screenshot({
          path: info.outputPath("settings-drawer.png"),
          animations: "disabled",
        });
      });
    });
  }
