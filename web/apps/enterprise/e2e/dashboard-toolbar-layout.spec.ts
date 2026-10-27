import { expect, test, type Page } from "@playwright/test";
import { selectTrigger } from "./helpers/select";

async function openPublishedDashboard(page: Page, english: boolean) {
  const tx = (zh: string, en: string) => (english ? en : zh);
  await page.goto("/login");
  await page.getByLabel(tx("用户名", "Username")).fill("root");
  await page.getByLabel(tx("密码", "Password")).fill("123456");
  await page
    .getByRole("button", { name: tx("登录", "Sign in"), exact: true })
    .click();
  await expect(page).not.toHaveURL(/login/);
  await page.goto("/dashboards");
  await page
    .getByRole("button", {
      name: tx("新建仪表盘", "Create dashboard"),
      exact: true,
    })
    .click();
  await page
    .getByRole("textbox", { name: tx("名称", "Name"), exact: true })
    .fill("Toolbar layout");
  await page
    .getByRole("button", { name: tx("完成", "Done"), exact: true })
    .click();
  await page
    .getByRole("button", { name: tx("添加统计图", "Add panel"), exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: tx("CPU 使用率", "CPU utilization"), exact: true })
    .click();
  await page
    .getByRole("button", { name: tx("完成", "Done"), exact: true })
    .click();
  await page
    .getByRole("button", {
      name: tx("预览并发布", "Preview and publish"),
      exact: true,
    })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", {
      name: tx("确认发布", "Confirm publication"),
      exact: true,
    })
    .click();
  await expect(page).toHaveURL(/\/dashboards\//);
}

for (const width of [1280, 1440, 1920]) {
  for (const english of [false, true]) {
    for (const theme of ["light", "dark"]) {
      test(`toolbar aligned ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(90_000);
        await page.setViewportSize({ width, height: 1000 });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await openPublishedDashboard(page, english);
        const tx = (zh: string, en: string) => (english ? en : zh);
        const toolbar = page.getByRole("group", {
          name: tx("仪表盘查询条件", "Dashboard query conditions"),
          exact: true,
        });
        const time = toolbar.getByRole("button", {
          name: tx("时间范围", "Time range"),
          exact: true,
        });
        const applications = toolbar.getByRole("button", {
          name: new RegExp(`^${tx("筛选应用", "Filter applications")}:`),
        });
        const resources = toolbar.getByRole("button", {
          name: new RegExp(`^${tx("选择资源", "Choose resources")}:`),
        });
        const type = selectTrigger(toolbar, tx("资源类型", "Resource type"));
        await expect(toolbar).toBeVisible();
        await expect(
          page.getByRole("heading", { name: "Toolbar layout", exact: true }),
        ).toBeVisible();
        await page.screenshot({
          path: info.outputPath("toolbar.png"),
          animations: "disabled",
        });
        const boxes = await Promise.all(
          [time, applications, type, resources].map((control) =>
            control.boundingBox(),
          ),
        );
        const bottom = boxes[0]!.y + boxes[0]!.height;
        for (const box of boxes) {
          expect(
            Math.abs(box!.y + box!.height - bottom),
            "filter controls should share a baseline",
          ).toBeLessThanOrEqual(1);
          expect(
            Math.abs(box!.height - boxes[0]!.height),
            "filter controls should have the same height",
          ).toBeLessThanOrEqual(1);
        }
        for (let i = 1; i < boxes.length; i++)
          expect(boxes[i]!.x).toBeGreaterThan(
            boxes[i - 1]!.x + boxes[i - 1]!.width,
          );
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        const refresh = toolbar.getByRole("button", {
          name: tx("刷新", "Refresh"),
          exact: true,
        });
        const interval = selectTrigger(
          toolbar,
          tx("刷新间隔", "Refresh interval"),
        );
        const [refreshBox, intervalBox] = await Promise.all([
          refresh.boundingBox(),
          interval.boundingBox(),
        ]);
        expect(
          Math.abs(
            refreshBox!.y +
              refreshBox!.height -
              intervalBox!.y -
              intervalBox!.height,
          ),
        ).toBeLessThanOrEqual(1);
        await applications.click();
        const applicationDialog = page.getByRole("dialog", {
          name: tx("筛选应用", "Filter applications"),
          exact: true,
        });
        await expect(applicationDialog).toBeVisible();
        await applicationDialog.press("Escape");
        await expect(applications).toBeFocused();
        await resources.click();
        const resourceDialog = page.getByRole("dialog", {
          name: tx("选择资源", "Choose resources"),
          exact: true,
        });
        await expect(resourceDialog).toBeVisible();
        await resourceDialog.press("Escape");
        await expect(resources).toBeFocused();
        await interval.click();
        await page
          .getByRole("option", {
            name: tx("自动刷新关闭", "Auto refresh off"),
            exact: true,
          })
          .click();
        await expect(interval).toContainText(
          tx("自动刷新关闭", "Auto refresh off"),
        );
        await time.click();
        const picker = page.getByRole("dialog", {
          name: tx("时间范围", "Time range"),
          exact: true,
        });
        await selectTrigger(picker, tx("常用范围", "Common ranges")).click();
        await page
          .getByRole("option", {
            name: tx("最近 24 小时", "Last 24 hours"),
            exact: true,
          })
          .click();
        await picker
          .getByRole("button", { name: tx("应用", "Apply"), exact: true })
          .click();
        await expect(time).toContainText(tx("最近 24 小时", "Last 24 hours"));
        await expect(page.locator("vite-error-overlay")).toHaveCount(0);
        expect(errors).toEqual([]);
        await page.screenshot({
          path: info.outputPath("toolbar-interaction.png"),
          animations: "disabled",
        });
      });
    }
  }
}
