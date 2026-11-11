import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";

const cluster = process.env.ARGUS_PLAN9_B10A_CLUSTER ?? "";
const base = "/api/v1/enterprise/kubernetes-clusters/" + cluster;
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
async function row(page: Page, namespace: string, prefix: RegExp) {
  await page.goto("/kubernetes/" + cluster + "?tab=resources");
  await selectWorkbenchNamespace(page, namespace);
  await page
    .getByRole("tablist", { name: /^(资源类型|Resource type)$/, exact: true })
    .getByRole("tab", { name: "Pod", exact: true })
    .click();
  const result = page
    .locator(".argus-k8s-workbench")
    .getByRole("row")
    .filter({ has: page.getByText(prefix) })
    .first();
  await expect(result).toBeVisible();
  return result;
}
async function footerLayout(page: Page, first: string, second: string) {
  await expect
    .poll(() =>
      page.getByRole("dialog").evaluate((element) => {
        for (
          let node: HTMLElement | null = element as HTMLElement;
          node && node !== document.body;
          node = node.parentElement
        ) {
          if (Number(getComputedStyle(node).opacity) < 0.999) return false;
        }
        return true;
      }),
    )
    .toBe(true);
  const footer = page.getByRole("dialog").locator(".argus-dialog__footer");
  const a = await footer
    .getByRole("button", { name: first, exact: true })
    .boundingBox();
  const b = await footer
    .getByRole("button", { name: second, exact: true })
    .boundingBox();
  expect(a && b).toBeTruthy();
  expect(Math.abs(a!.y - b!.y)).toBeLessThan(2);
  expect(b!.x - a!.x - a!.width).toBeGreaterThanOrEqual(8);
}
test.describe("B10a quiet Collector logs and session layout", () => {
  test.skip(
    process.env.ARGUS_PLAN9_B10A_E2E !== "1",
    "Requires the owned real B10a environment",
  );
  test.describe.configure({ mode: "serial", timeout: 90000 });
  test.beforeEach(async ({ context }) => {
    expect(cluster).not.toBe("");
    const value = JSON.parse(
      readFileSync(process.env.ARGUS_PLAN9_SESSION_PATH!, "utf8"),
    );
    await context.addCookies(
      value.cookies.map((cookie: object) => ({
        ...cookie,
        httpOnly: true,
        sameSite: "Lax",
      })),
    );
  });
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`quiet retained logs and aligned actions ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        const text = (zh: string, en: string) => (english ? en : zh);
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.setViewportSize({
          width: theme === "light" ? 1280 : 1920,
          height: 1100,
        });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        const agent = await row(
          page,
          "argus-telemetry",
          /^argus-otelcol-agent-/,
        );
        const created = page.waitForResponse(
          (r) =>
            new URL(r.url()).pathname === base + "/log-sessions" &&
            r.request().method() === "POST",
        );
        const firstRead = page.waitForResponse(
          (r) =>
            new URL(r.url()).pathname.startsWith(base + "/log-sessions/") &&
            new URL(r.url()).pathname.endsWith("/read") &&
            r.request().method() === "POST",
        );
        await agent
          .getByRole("button", {
            name: text("查看日志", "View logs"),
            exact: true,
          })
          .click();
        const response = await created;
        expect(response.status()).toBe(201);
        expect(response.request().postDataJSON().since_seconds).toBe(0);
        const ticket = await response.json();
        const id = ticket.session.id;
        ticket.ticket = "";
        const read = await firstRead;
        expect(read.status()).toBe(200);
        const batch = await read.json();
        expect(batch.content.length).toBeGreaterThan(0);
        const stamps = String(batch.content)
          .split("\n")
          .map((line) => Date.parse(line.split(" ")[0]!))
          .filter(Number.isFinite);
        expect(stamps.some((stamp) => Date.now() - stamp > 300000)).toBe(true);
        const dialog = page.getByRole("dialog", {
          name: /^(Pod 日志|Pod logs)/,
        });
        const output = dialog.getByRole("log");
        await expect(output).toContainText(
          /Starting scrape manager|Everything is ready/,
          { timeout: 20000 },
        );
        const container = await dialog
          .getByRole("button", {
            name: text("容器", "Container"),
            exact: true,
          })
          .boundingBox();
        const options = await dialog
          .getByRole("button", {
            name: text("日志选项", "Log options"),
            exact: true,
          })
          .boundingBox();
        expect(
          Math.abs(
            container!.y + container!.height - options!.y - options!.height,
          ),
        ).toBeLessThan(2);
        const viewer = await output.boundingBox();
        const controls = await dialog
          .locator(".argus-k8s-log-controls")
          .boundingBox();
        expect(viewer!.y - controls!.y - controls!.height).toBeGreaterThan(16);
        expect(
          (await new AxeBuilder({ page }).analyze()).violations.filter(
            (v) => v.impact === "critical" || v.impact === "serious",
          ),
        ).toEqual([]);
        await page.screenshot({
          path: info.outputPath("actual-quiet-agent-history.png"),
          fullPage: true,
        });
        const closing = page.waitForResponse(
          (r) =>
            new URL(r.url()).pathname ===
            base + "/log-sessions/" + id + "/close",
        );
        await dialog
          .getByRole("button", {
            name: text("关闭日志会话", "Close log session"),
            exact: true,
          })
          .click();
        expect((await closing).status()).toBe(200);
        await expect(dialog).not.toBeVisible();
        await agent
          .getByRole("button", {
            name: text("打开终端", "Open terminal"),
            exact: true,
          })
          .click();
        const terminal = page.getByRole("dialog", {
          name: /^(Pod 终端|Pod terminal)/,
        });
        await expect(
          terminal.getByRole("region", {
            name: text("普通终端支持条件", "Terminal requirements"),
          }),
        ).toContainText("Collector");
        await footerLayout(
          page,
          text("关闭会话", "Close session"),
          text("创建会话", "Create session"),
        );
        await page.screenshot({
          path: info.outputPath("terminal-form-aligned-footer.png"),
          fullPage: true,
        });
        await terminal
          .getByRole("textbox", {
            name: text("操作原因", "Reason"),
            exact: true,
          })
          .fill(
            "Verify explicit support limits for the reported quiet Collector",
          );
        await terminal
          .getByRole("button", {
            name: text("创建会话", "Create session"),
            exact: true,
          })
          .click();
        await expect(terminal.getByRole("alert")).toContainText("pods/exec", {
          timeout: 15000,
        });
        await footerLayout(
          page,
          text("关闭会话", "Close session"),
          text("重新连接", "Reconnect"),
        );
        await terminal
          .getByRole("button", {
            name: text("关闭会话", "Close session"),
            exact: true,
          })
          .click();
        await expect(terminal).not.toBeVisible();
        expect(errors).toEqual([]);
      });
    }
  test("supported business Pod runs a real shell and can reconnect", async ({
    page,
  }, info) => {
    await page.setViewportSize({ width: 1440, height: 1100 });
    await page.addInitScript(() => {
      localStorage.setItem("argus.locale", "zh-CN");
      localStorage.setItem("argus.theme", "light");
    });
    const pod = await row(page, "b10a-workloads", /^b10a-api-/);
    await pod.getByRole("button", { name: "打开终端", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: /^Pod 终端/ });
    const outputs: string[] = [];
    page.on("websocket", (socket) =>
      socket.on("framereceived", (event) => {
        try {
          const frame = JSON.parse(String(event.payload));
          if (frame.type === "output")
            outputs.push(Buffer.from(frame.data, "base64").toString());
        } catch {
          /* other traffic */
        }
      }),
    );
    await dialog
      .getByRole("textbox", { name: "操作原因", exact: true })
      .fill("Validate supported business container");
    await dialog.getByRole("button", { name: "创建会话", exact: true }).click();
    const command = async () => {
      await expect(dialog.getByText("已连接", { exact: true })).toBeVisible({
        timeout: 35000,
      });
      await dialog.locator(".xterm-helper-textarea").focus();
      await page.keyboard.type("cat /config/mode");
      await page.keyboard.press("Enter");
      await expect.poll(() => outputs.join("")).toContain("updated");
    };
    await command();
    await page.waitForTimeout(5000);
    outputs.length = 0;
    await command();
    await page.screenshot({
      path: info.outputPath("actual-business-terminal.png"),
      fullPage: true,
    });
    await page.keyboard.type("exit");
    await page.keyboard.press("Enter");
    await expect(
      dialog.getByRole("button", { name: "重新连接", exact: true }),
    ).toBeVisible({ timeout: 10000 });
    await dialog.getByRole("button", { name: "重新连接", exact: true }).click();
    outputs.length = 0;
    await command();
    await dialog.getByRole("button", { name: "关闭会话", exact: true }).click();
    await expect(dialog).not.toBeVisible();
  });
});
