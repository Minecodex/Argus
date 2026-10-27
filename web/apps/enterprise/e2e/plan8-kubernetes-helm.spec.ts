import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { gzipSync } from "node:zlib";

// Valid packaged chart fixture. The portal test uses mock mode; native Helm
// installation and actual object assertions have a separate real-cluster gate.
function chartArchive() {
  const chunks: Buffer[] = [];
  for (const [name, content] of Object.entries({
    "sample/Chart.yaml": "apiVersion: v2\nname: sample\nversion: 1.0.0\n",
    "sample/templates/settings.yaml":
      "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: '{{ .Release.Name }}-settings'\ndata:\n  message: '{{ .Values.message }}'\n",
  })) {
    const raw = Buffer.from(content),
      header = Buffer.alloc(512);
    header.write(name);
    header.write("0000600\0", 100);
    header.write("0000000\0", 108);
    header.write("0000000\0", 116);
    header.write(raw.length.toString(8).padStart(11, "0") + "\0", 124);
    header.write("00000000000\0", 136);
    header.fill(32, 148, 156);
    header[156] = 48;
    header.write("ustar\0", 257);
    header.write("00", 263);
    header.write(
      header
        .reduce((sum, value) => sum + value, 0)
        .toString(8)
        .padStart(6, "0") + "\0 ",
      148,
    );
    chunks.push(header, raw, Buffer.alloc((512 - (raw.length % 512)) % 512));
  }
  return gzipSync(Buffer.concat([...chunks, Buffer.alloc(1024)]));
}
async function login(page: Page, user = "root") {
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill(user);
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}
async function logout(page: Page, user: string, english: boolean) {
  await page.getByRole("button", { name: new RegExp(`${user}$`) }).click();
  await page
    .getByRole("menuitem", {
      name: english ? "Sign out" : "退出登录",
      exact: true,
    })
    .click();
  await expect(page).toHaveURL(/\/login/);
}

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 Helm lifecycle ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(120000);
      const text = (zh: string, en: string) => (english ? en : zh);
      const errors: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.setViewportSize({ width: 1440, height: 1100 });
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await login(page);
      const open = async () => {
        await page.goto("/kubernetes/k8s-prod-east");
        await page
          .getByRole("button", {
            name: text("Helm 发布管理", "Helm releases"),
            exact: true,
          })
          .click();
        const dialog = page.getByRole("dialog", {
          name: text("Helm 发布管理", "Helm releases"),
          exact: true,
        });
        await dialog
          .getByRole("button", {
            name: text("选择命名空间", "Choose a namespace"),
            exact: true,
          })
          .click();
        await page
          .getByRole("dialog", {
            name: text("选择命名空间", "Choose a namespace"),
            exact: true,
          })
          .getByRole("button", { name: /^(Select|选择) production$/ })
          .click();
        return dialog;
      };
      const preview = async (operation: string, needsChart = false) => {
        const dialog = page.getByRole("dialog", {
          name: `${operation} Helm ${english ? "release" : "发布"}`,
          exact: true,
        });
        if (needsChart) {
          await dialog
            .locator('input[type="file"]')
            .setInputFiles({
              name: "sample-1.0.0.tgz",
              mimeType: "application/gzip",
              buffer: chartArchive(),
            });
          await dialog
            .getByRole("textbox", { name: /values/ })
            .fill(
              "message: sample\npassword: plan8-helm-private-portal-canary",
            );
        }
        await dialog
          .getByRole("textbox", {
            name: new RegExp(text("操作原因", "Reason")),
          })
          .fill("Validate owned Helm lifecycle");
        await dialog
          .getByRole("button", {
            name: text("验证并预览差异", "Validate and preview"),
            exact: true,
          })
          .click();
        await expect(dialog).toContainText("ConfigMap");
        await expect(dialog).not.toContainText(
          "plan8-helm-private-portal-canary",
        );
        expect(
          await page.evaluate(() => JSON.stringify(localStorage)),
        ).not.toContain("plan8-helm-private-portal-canary");
        return dialog;
      };
      const confirm = async (dialog: ReturnType<Page["getByRole"]>) => {
        await dialog
          .getByRole("button", {
            name: text("确认执行", "Confirm operation"),
            exact: true,
          })
          .click();
        const history = page.getByRole("dialog", {
          name: text("操作记录", "Operation history"),
          exact: true,
        });
        await expect(
          history
            .getByRole("row")
            .filter({ hasText: "plan8-helm-portal" })
            .first(),
        ).toContainText(text("已生效", "Applied"));
        await page.keyboard.press("Escape");
      };
      let list = await open();
      await list
        .getByRole("button", { name: text("安装", "Install"), exact: true })
        .click();
      await page
        .getByRole("textbox", {
          name: new RegExp(text("发布名称", "Release name")),
        })
        .fill("plan8-helm-portal");
      let change = await preview(text("安装", "Install"), true);
      expect(
        (await new AxeBuilder({ page }).analyze()).violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("helm-install-preview.png"),
        fullPage: true,
      });
      await confirm(change);
      list = await open();
      await list
        .getByRole("button", { name: text("升级", "Upgrade"), exact: true })
        .click();
      change = await preview(text("升级", "Upgrade"), true);
      await confirm(change);
      list = await open();
      await list
        .getByRole("button", { name: text("版本历史", "History"), exact: true })
        .click();
      await list
        .getByRole("row")
        .filter({ has: page.getByRole("cell", { name: "1", exact: true }) })
        .getByRole("button", { name: text("回滚", "Rollback"), exact: true })
        .click();
      change = await preview(text("回滚", "Rollback"));
      await expect(change).toContainText(
        text("回滚目标版本", "Rollback revision"),
      );
      await confirm(change);
      list = await open();
      await expect(
        list.getByRole("row").filter({ hasText: "plan8-helm-portal" }).first(),
      ).toContainText("3");
      await list
        .getByRole("button", { name: text("卸载", "Uninstall"), exact: true })
        .click();
      change = await preview(text("卸载", "Uninstall"));
      await change
        .getByRole("button", {
          name: text("确认执行", "Confirm operation"),
          exact: true,
        })
        .click();
      await expect(change).toContainText(
        english ? "dual approval" : "等待审批",
      );
      await page.goto("/approvals");
      await logout(page, "root", english);
      await login(page, "chenxi");
      await page.goto("/approvals");
      await page.getByRole("button", { name: /plan8-helm-portal/ }).click();
      const detail = page.locator(".argus-approval-detail");
      await detail
        .getByLabel(text("批准", "Approve"), { exact: true })
        .fill("Reviewed owned Helm uninstall");
      await detail
        .getByRole("button", { name: text("批准", "Approve"), exact: true })
        .click();
      await logout(page, "chenxi", english);
      await login(page);
      list = await open();
      await expect(
        list.getByRole("row").filter({ hasText: "plan8-helm-portal" }).first(),
      ).toContainText(text("已卸载", "Uninstalled"));
      expect(errors).toEqual([]);
      await page.screenshot({
        path: info.outputPath("helm-uninstalled-history.png"),
        fullPage: true,
      });
    });
  }
