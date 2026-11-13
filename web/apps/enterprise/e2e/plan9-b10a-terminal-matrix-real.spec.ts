import { selectRowAction } from "./helpers/action-menu";
import { readFileSync, writeFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";
const inventory = process.env.ARGUS_PLAN9_POD_AUDIT_FILE
  ? JSON.parse(readFileSync(process.env.ARGUS_PLAN9_POD_AUDIT_FILE, "utf8"))
  : { pods: [] };
const candidates = inventory.pods.filter(
  (pod: { preflight: string }) => pod.preflight === "shell_present",
) as { pod: string; pod_uid: string; container: string; namespace: string }[];
const cluster = process.env.ARGUS_PLAN9_B10A_CLUSTER ?? "";
const base = "/api/v1/enterprise/kubernetes-clusters/" + cluster;
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
async function resource(page: Page, namespace: string, pod: string | RegExp) {
  await page.goto("/kubernetes/" + cluster + "?tab=resources");
  await selectWorkbenchNamespace(page, namespace);
  await page
    .getByRole("tablist", { name: /^(资源类型|Resource type)$/, exact: true })
    .getByRole("tab", { name: "Pod", exact: true })
    .click();
  const row = page
    .locator(".argus-k8s-workbench")
    .getByRole("row")
    .filter({ has: page.getByText(pod, { exact: typeof pod === "string" }) })
    .first();
  await expect(row).toBeVisible();
  return row;
}
test.describe("B10a current terminal matrix and expanded log options", () => {
  test.skip(
    process.env.ARGUS_PLAN9_B10A_E2E !== "1",
    "Requires owned B10a runtime",
  );
  test.describe.configure({ mode: "serial", timeout: 90000 });
  test.beforeEach(async ({ context }) => {
    expect(cluster).not.toBe("");
    expect(candidates).toHaveLength(4);
    const session = JSON.parse(
      readFileSync(process.env.ARGUS_PLAN9_SESSION_PATH!, "utf8"),
    );
    await context.addCookies(
      session.cookies.map((cookie: object) => ({
        ...cookie,
        httpOnly: true,
        sameSite: "Lax",
      })),
    );
  });
  for (const pod of candidates) {
    test("actual terminal " + pod.pod, async ({ page }, info) => {
      await page.setViewportSize({ width: 1440, height: 1100 });
      await page.addInitScript(() => {
        localStorage.setItem("argus.locale", "zh-CN");
        localStorage.setItem("argus.theme", "light");
      });
      const row = await resource(page, pod.namespace, pod.pod);
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
        .fill("Audit current B10a business Pod terminal support");
      const creating = page.waitForResponse(
        (r) =>
          new URL(r.url()).pathname === base + "/stream-sessions" &&
          r.request().method() === "POST",
      );
      await dialog
        .getByRole("button", { name: "创建会话", exact: true })
        .click();
      const response = await creating;
      expect(response.status()).toBe(201);
      const value = await response.json();
      value.ticket = "";
      expect(value.session.pod_uid).toBe(pod.pod_uid);
      await expect(dialog.getByText("已连接", { exact: true })).toBeVisible({
        timeout: 30000,
      });
      await dialog.locator(".xterm-helper-textarea").focus();
      await page.keyboard.type("id -u; command -v sh; command -v bash; pwd");
      await page.keyboard.press("Enter");
      await expect.poll(() => output.join("")).toContain("10001");
      await expect.poll(() => output.join("")).toMatch(/\/(usr\/)?bin\/sh/);
      await page.screenshot({
        path: info.outputPath("actual-supported-terminal.png"),
        fullPage: true,
      });
      await dialog
        .getByRole("button", { name: "关闭会话", exact: true })
        .click();
      await expect(dialog).not.toBeVisible();
      let final: { status: string; bytes_in: number; bytes_out: number };
      await expect
        .poll(async () => {
          const r = await page.request.get(
            base + "/stream-sessions/" + value.session.id,
          );
          final = await r.json();
          return final.status;
        })
        .toBe("closed");
      expect(final!.bytes_in).toBeGreaterThan(0);
      expect(final!.bytes_out).toBeGreaterThan(0);
      writeFileSync(
        info.outputPath("verified-session.json"),
        JSON.stringify(
          {
            namespace: pod.namespace,
            pod: pod.pod,
            container: pod.container,
            shell: value.session.shell,
            pod_uid: pod.pod_uid,
            session_id: value.session.id,
            status: final!.status,
            bytes_in: final!.bytes_in,
            bytes_out: final!.bytes_out,
            real_uid_output: true,
          },
          null,
          2,
        ),
      );
    });
  }
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`expanded log options align ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        const text = (zh: string, en: string) => (english ? en : zh);
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
        const row = await resource(
          page,
          "argus-telemetry",
          /^argus-otelcol-agent-/,
        );
        await selectRowAction(row, text("查看日志", "View logs"));
        const dialog = page.getByRole("dialog", {
          name: /^(Pod 日志|Pod logs)/,
        });
        await expect(dialog.getByRole("log")).toContainText(
          "Starting scrape manager",
          { timeout: 15000 },
        );
        await dialog
          .getByRole("button", {
            name: text("日志选项", "Log options"),
            exact: true,
          })
          .click();
        const tail = dialog.getByRole("spinbutton", {
          name: text("初始末尾行数", "Initial tail lines"),
          exact: true,
        });
        const window = dialog.getByRole("spinbutton", {
          name: text("初始时间窗（秒）", "Initial window (seconds)"),
          exact: true,
        });
        const apply = dialog.getByRole("button", {
          name: text("应用并重新读取", "Apply and reload"),
          exact: true,
        });
        const [a, b, c] = await Promise.all([
          tail.boundingBox(),
          window.boundingBox(),
          apply.boundingBox(),
        ]);
        expect(a && b && c).toBeTruthy();
        expect(Math.abs(a!.y - b!.y)).toBeLessThan(2);
        expect(Math.abs(a!.y - c!.y)).toBeLessThan(2);
        expect(c!.x - b!.x - b!.width).toBeGreaterThanOrEqual(8);
        const hint = dialog.getByText(
          text(
            "0 表示不限制时间，按最近行数读取。",
            "Use 0 to read the latest retained lines without a time filter.",
          ),
          { exact: true },
        );
        const h = await hint.boundingBox();
        expect(h!.y).toBeGreaterThanOrEqual(b!.y + b!.height);
        expect(
          (await new AxeBuilder({ page }).analyze()).violations.filter(
            (v) => v.impact === "critical" || v.impact === "serious",
          ),
        ).toEqual([]);
        await page.screenshot({
          path: info.outputPath("expanded-options-aligned.png"),
          fullPage: true,
        });
        await dialog
          .getByRole("button", {
            name: text("关闭日志会话", "Close log session"),
            exact: true,
          })
          .click();
        await expect(dialog).not.toBeVisible();
      });
    }
});
