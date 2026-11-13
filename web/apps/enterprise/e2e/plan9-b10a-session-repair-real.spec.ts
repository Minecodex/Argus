import { selectRowAction } from "./helpers/action-menu";
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import { resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";

const cluster = process.env.ARGUS_PLAN9_B10A_CLUSTER ?? "";
const root = resolve(import.meta.dirname, "../../../..");
const base = "/api/v1/enterprise/kubernetes-clusters/" + cluster;
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
function actualRequest(marker: string) {
  const code =
    "import urllib.request;urllib.request.urlopen('http://b10a-api:8080/?label=" +
    marker +
    "',timeout=5).read()";
  const script =
    "import sys;sys.path.insert(0,'docs/plan9/runtime/B10a');from run import Run;r=Run();r.kubectl('exec','-n','b10a-workloads','deployment/b10a-api','--','python3','-c'," +
    JSON.stringify(code) +
    ")";
  execFileSync("python", ["-c", script], {
    cwd: root,
    timeout: 20000,
    stdio: "pipe",
  });
}
async function podRow(page: Page, ns: string, prefix: RegExp) {
  await page.goto("/kubernetes/" + cluster + "?tab=resources");
  await selectWorkbenchNamespace(page, ns);
  await page
    .getByRole("tablist", { name: /^(资源类型|Resource type)$/, exact: true })
    .getByRole("tab", { name: "Pod", exact: true })
    .click();
  const row = page
    .locator(".argus-k8s-workbench")
    .getByRole("row")
    .filter({ has: page.getByText(prefix) })
    .first();
  await expect(row).toBeVisible();
  return row;
}
test.describe("B10a deployed automatic logs and terminal diagnosis", () => {
  test.skip(
    process.env.ARGUS_PLAN9_B10A_E2E !== "1",
    "Requires the owned live B10a environment",
  );
  test.describe.configure({ mode: "serial", timeout: 120000 });
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
      test(`logs open directly and follow actual output ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        const text = (zh: string, en: string) => (english ? en : zh);
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.setViewportSize({
          width: theme === "light" ? 1280 : 1920,
          height: 1050,
        });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        const row = await podRow(page, "b10a-workloads", /^b10a-api-/);
        const created = page.waitForResponse(
          (r) =>
            new URL(r.url()).pathname === base + "/log-sessions" &&
            r.request().method() === "POST",
        );
        await selectRowAction(row, text("查看日志", "View logs"));
        const dialog = page.getByRole("dialog", {
          name: /^(Pod 日志|Pod logs)/,
        });
        const ticket = await (await created).json();
        const id = ticket.session.id;
        ticket.ticket = "";
        const output = dialog.getByRole("log");
        const previousLabel = dialog.locator(
          ".argus-k8s-log-controls > .argus-k8s-workbench-controls > span",
        );
        await expect(previousLabel).toHaveText(
          text(
            "读取上次终止容器日志",
            "Read the previously terminated container",
          ),
        );
        await expect(previousLabel).toBeVisible();
        await expect(output).toContainText("p9-b10a", { timeout: 30000 });
        await expect(
          dialog.getByRole("button", {
            name: text("打开日志会话", "Open log session"),
            exact: true,
          }),
        ).toHaveCount(0);
        await expect
          .poll(() => output.evaluate((e) => e.scrollHeight - e.clientHeight))
          .toBeGreaterThan(100);
        const follow = dialog.getByRole("switch", {
          name: text("自动滚动", "Follow"),
          exact: true,
        });
        await expect(follow).toBeChecked();
        await output.hover();
        await page.mouse.wheel(0, -500);
        await expect(follow).not.toBeChecked();
        const top = await output.evaluate((e) => e.scrollTop);
        const marker = "scroll-" + randomUUID();
        actualRequest(marker);
        await expect(output).toContainText(marker, { timeout: 12000 });
        expect(
          Math.abs((await output.evaluate((e) => e.scrollTop)) - top),
        ).toBeLessThan(3);
        await page.screenshot({
          path: info.outputPath("logs-history-position.png"),
          fullPage: true,
        });
        await output.hover();
        await page.mouse.wheel(0, 50000);
        await expect(follow).toBeChecked();
        const latest = "latest-" + randomUUID();
        actualRequest(latest);
        await expect(output).toContainText(latest, { timeout: 12000 });
        await expect
          .poll(() =>
            output.evaluate(
              (e) => e.scrollHeight - e.scrollTop - e.clientHeight,
            ),
          )
          .toBeLessThan(3);
        expect(
          (await new AxeBuilder({ page }).analyze()).violations.filter(
            (v) => v.impact === "critical" || v.impact === "serious",
          ),
        ).toEqual([]);
        await page.screenshot({
          path: info.outputPath("automatic-live-logs.png"),
          fullPage: true,
        });
        const closing = page.waitForResponse(
          (response) =>
            new URL(response.url()).pathname ===
              base + "/log-sessions/" + id + "/close" &&
            response.request().method() === "POST",
        );
        await dialog
          .getByRole("button", {
            name: text("关闭日志会话", "Close log session"),
            exact: true,
          })
          .click();
        await expect(dialog).not.toBeVisible();
        const closed = await closing;
        expect(closed.status()).toBe(200);
        expect((await closed.json()).status).toBe("closed");
        expect(errors).toEqual([]);
      });
    }
  test("real Pod terminal stays active, executes commands and reconnects after exit", async ({
    page,
  }, info) => {
    await page.setViewportSize({ width: 1440, height: 1050 });
    await page.addInitScript(() => {
      localStorage.setItem("argus.locale", "zh-CN");
      localStorage.setItem("argus.theme", "light");
    });
    const row = await podRow(page, "b10a-workloads", /^b10a-api-/);
    await selectRowAction(row, "打开终端");
    const dialog = page.getByRole("dialog", { name: /^Pod 终端/ });
    const output: string[] = [];
    page.on("websocket", (socket) =>
      socket.on("framereceived", (event) => {
        try {
          const frame = JSON.parse(String(event.payload));
          if (frame.type === "output")
            output.push(Buffer.from(frame.data, "base64").toString());
        } catch {
          /* unrelated traffic */
        }
      }),
    );
    await dialog
      .getByRole("textbox", { name: "操作原因", exact: true })
      .fill("Verify B10a real terminal after log feedback");
    const created = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === base + "/stream-sessions" &&
        r.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "创建会话", exact: true }).click();
    const first = await (await created).json();
    first.ticket = "";
    await expect(dialog.getByText("已连接", { exact: true })).toBeVisible({
      timeout: 35000,
    });
    const command = async () => {
      await dialog.locator(".xterm-helper-textarea").focus();
      await page.keyboard.type('cat /config/mode; printf "\\n"; pwd');
      await page.keyboard.press("Enter");
      await expect.poll(() => output.join("")).toContain("updated");
    };
    await command();
    for (let i = 0; i < 4; i++) {
      await expect
        .poll(async () => {
          const r = await page.request.get(
            base + "/stream-sessions/" + first.session.id,
          );
          return r.ok() ? (await r.json()).status : "error";
        })
        .toBe("active");
      await page.waitForTimeout(3000);
    }
    output.length = 0;
    await command();
    await page.screenshot({
      path: info.outputPath("actual-terminal-output.png"),
      fullPage: true,
    });
    await dialog.locator(".xterm-helper-textarea").focus();
    await page.keyboard.type("exit");
    await page.keyboard.press("Enter");
    await expect(
      dialog.getByRole("button", { name: "重新连接", exact: true }),
    ).toBeVisible({ timeout: 10000 });
    const next = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === base + "/stream-sessions" &&
        r.request().method() === "POST",
    );
    await dialog.getByRole("button", { name: "重新连接", exact: true }).click();
    const second = await (await next).json();
    second.ticket = "";
    expect(second.session.id).not.toBe(first.session.id);
    await expect(dialog.getByText("已连接", { exact: true })).toBeVisible({
      timeout: 35000,
    });
    output.length = 0;
    await command();
    await dialog.getByRole("button", { name: "关闭会话", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    for (const id of [first.session.id, second.session.id])
      await expect
        .poll(async () => {
          const r = await page.request.get(base + "/stream-sessions/" + id);
          return r.ok() ? (await r.json()).status : "error";
        })
        .toBe("closed");
  });
  test("the reported Collector Pod shows the actual native permission refusal", async ({
    page,
  }, info) => {
    await page.setViewportSize({ width: 1440, height: 1050 });
    await page.addInitScript(() => {
      localStorage.setItem("argus.locale", "zh-CN");
      localStorage.setItem("argus.theme", "light");
    });
    const row = await podRow(page, "argus-telemetry", /^argus-otelcol-agent-/);
    await selectRowAction(row, "打开终端");
    const dialog = page.getByRole("dialog", { name: /^Pod 终端/ });
    await expect(dialog).toContainText("不含 Shell");
    await dialog
      .getByRole("textbox", { name: "操作原因", exact: true })
      .fill("Verify the exact Collector Pod refusal without changing access");
    await dialog.getByRole("button", { name: "创建会话", exact: true }).click();
    await expect(dialog.getByRole("alert")).toContainText("pods/exec", {
      timeout: 15000,
    });
    await expect(dialog.getByText("连接已中断", { exact: true })).toBeVisible();
    await expect(dialog.locator(".xterm-helper-textarea")).toHaveCount(0);
    await page.screenshot({
      path: info.outputPath("collector-native-access-denied.png"),
      fullPage: true,
    });
    await dialog.getByRole("button", { name: "关闭会话", exact: true }).click();
    await expect(dialog).not.toBeVisible();
  });
});
