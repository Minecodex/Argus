import { readFileSync } from "node:fs";
import { expect, test, type Page, type Locator } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
const cluster = process.env.ARGUS_PLAN9_B10A_CLUSTER ?? "";
const namespace = "b10a-workloads";
const base = `/api/v1/enterprise/kubernetes-clusters/${cluster}`;

async function expectAppliedOperation(
  page: Page,
  history: Locator,
  name: string,
  english: boolean,
  operation: "create" | "patch",
) {
  await expect
    .poll(
      async () => {
        const response = await page.request.get(base + "/operations?limit=200");
        if (!response.ok()) return false;
        const value = await response.json();
        return value.items.some(
          (row: { name: string; status: string; operation: string }) =>
            row.name === name &&
            row.operation === operation &&
            row.status === "succeeded",
        );
      },
      { timeout: 120_000 },
    )
    .toBe(true);
  await page.waitForResponse((response) => {
    const url = new URL(response.url());
    return (
      url.pathname.endsWith("/operations") &&
      url.searchParams.get("limit") === "20" &&
      response.status() === 200
    );
  });
  const pagination = history.getByRole("navigation", {
    name: english ? "Pagination" : "分页",
    exact: true,
  });
  for (let index = 0; index < 10; index++) {
    await expect(pagination).not.toHaveAttribute("aria-busy", "true");
    const row = history
      .getByRole("row")
      .filter({ has: page.getByText(name, { exact: true }) })
      .filter({
        has: page.getByRole("cell", {
          name:
            operation === "create"
              ? english
                ? "Create"
                : "创建"
              : english
                ? "Edit"
                : "编辑",
          exact: true,
        }),
      })
      .first();
    if (await row.count()) {
      await expect(row).toContainText(english ? "Applied" : "已生效", {
        timeout: 100_000,
      });
      return;
    }
    const next = history.getByRole("button", {
      name: english ? "Next page" : "下一页",
      exact: true,
    });
    if (!(await next.isEnabled())) break;
    const loaded = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname.endsWith("/operations") &&
        response.request().method() === "GET",
    );
    await next.click();
    expect((await loaded).status()).toBe(200);
    await expect(pagination).not.toHaveAttribute("aria-busy", "true");
  }
  throw new Error(
    "Applied operation was not visible through the actual paginated history",
  );
}

test.describe("B10a real workbench acceptance", () => {
  test.skip(
    process.env.ARGUS_PLAN9_B10A_E2E !== "1",
    "Requires the owned B10a cluster and normal enterprise login",
  );
  test.describe.configure({ mode: "serial", timeout: 240_000 });
  test.beforeEach(async ({ context }) => {
    expect(cluster).not.toBe("");
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
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`objects and actual ConfigMap lifecycle ${english ? "en" : "zh"} ${theme}`, async ({
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
        await page.goto(`/kubernetes/${cluster}?tab=resources`);
        await selectWorkbenchNamespace(page, namespace);
        const workbench = page.locator(".argus-k8s-workbench");
        const categories = page.getByRole("tablist", {
          name: text("资源类别", "Resource category"),
          exact: true,
        });
        const types = page.getByRole("tablist", {
          name: text("资源类型", "Resource type"),
          exact: true,
        });
        for (const [kind, object] of [
          ["Deployment", "b10a-api"],
          ["StatefulSet", "b10a-ledger"],
          ["DaemonSet", "b10a-node-observer"],
        ]) {
          await types.getByRole("tab", { name: kind, exact: true }).click();
          await expect(
            workbench
              .getByRole("row")
              .filter({ has: page.getByText(object!, { exact: true }) }),
          ).toBeVisible();
        }
        await categories
          .getByRole("tab", { name: text("网络", "Networking"), exact: true })
          .click();
        for (const [kind, object] of [
          ["Service", "b10a-api"],
          ["Ingress", "b10a-http"],
        ]) {
          await types.getByRole("tab", { name: kind, exact: true }).click();
          await expect(
            workbench
              .getByRole("row")
              .filter({ has: page.getByText(object!, { exact: true }) }),
          ).toBeVisible();
        }
        await categories
          .getByRole("tab", {
            name: text("配置", "Configuration"),
            exact: true,
          })
          .click();
        await types.getByRole("tab", { name: "Secret", exact: true }).click();
        const secret = workbench
          .getByRole("row")
          .filter({ has: page.getByText("b10a-demo-secret", { exact: true }) });
        await secret
          .getByRole("button", {
            name: text("查看对象", "View object"),
            exact: true,
          })
          .click();
        await expect(page.getByRole("dialog").locator("pre")).toContainText(
          "[REDACTED]",
        );
        await page.screenshot({
          path: info.outputPath("secret-projection.png"),
          fullPage: true,
        });
        await page.keyboard.press("Escape");
        await types
          .getByRole("tab", { name: "ConfigMap", exact: true })
          .click();
        const name = `b10a-ui-${english ? "en" : "zh"}-${theme}-${Date.now().toString().slice(-6)}`;
        await workbench
          .getByRole("button", {
            name: text("新建对象", "Create object"),
            exact: true,
          })
          .click();
        const dialog = page.getByRole("dialog", {
          name: text("新建 ConfigMap", "Create ConfigMap"),
          exact: true,
        });
        const manifest = dialog.getByRole("textbox", {
          name: text(
            "对象清单（YAML / JSON）",
            "Object manifest (YAML / JSON)",
          ),
          exact: true,
        });
        await manifest.fill(
          JSON.stringify({
            apiVersion: "v1",
            kind: "ConfigMap",
            metadata: { name, namespace, labels: { plan9_batch: "b10a" } },
            data: { mode: "browser-created" },
          }),
        );
        await dialog
          .getByRole("button", {
            name: text("验证并预览差异", "Validate and preview"),
            exact: true,
          })
          .click();
        await expect(dialog).toContainText("browser-created");
        await page.screenshot({
          path: info.outputPath("real-change-preview.png"),
          fullPage: true,
        });
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
        await expectAppliedOperation(page, history, name, english, "create");
        await history
          .getByRole("button", { name: text("关闭", "Close"), exact: true })
          .click();
        await expect(history).not.toBeVisible();
        await workbench
          .getByRole("button", { name: text("刷新", "Refresh"), exact: true })
          .click();
        const row = workbench
          .getByRole("row")
          .filter({ has: page.getByText(name, { exact: true }) });
        await expect(row).toBeVisible();
        await row
          .getByRole("button", {
            name: text("管理对象", "Manage object"),
            exact: true,
          })
          .click();
        const edit = page.getByRole("dialog", {
          name: text("管理 ConfigMap · ", "Manage ConfigMap · ") + name,
          exact: true,
        });
        await edit
          .getByRole("textbox", {
            name: text(
              "对象清单（YAML / JSON）",
              "Object manifest (YAML / JSON)",
            ),
            exact: true,
          })
          .fill('{"data":{"mode":"browser-updated"}}');
        await edit
          .getByRole("button", {
            name: text("验证并预览差异", "Validate and preview"),
            exact: true,
          })
          .click();
        await expect(edit).toContainText("browser-updated");
        await edit
          .getByRole("button", {
            name: text("确认执行", "Confirm operation"),
            exact: true,
          })
          .click();
        await expectAppliedOperation(page, history, name, english, "patch");
        await history
          .getByRole("button", { name: text("关闭", "Close"), exact: true })
          .click();
        await expect(history).not.toBeVisible();
        await workbench
          .getByRole("button", { name: text("刷新", "Refresh"), exact: true })
          .click();
        await row
          .getByRole("button", {
            name: text("查看对象", "View object"),
            exact: true,
          })
          .click();
        await expect(page.getByRole("dialog").locator("pre")).toContainText(
          "browser-updated",
        );
        await page.keyboard.press("Escape");
        const scope = await page.request.get(
          base + "/inventory?type=namespace&limit=100",
        );
        expect(scope.status()).toBe(200);
        const directory = await scope.json();
        const ns = directory.items.find(
          (item: { name: string }) => item.name === namespace,
        );
        const namespaceId = directory.namespace_references[ns.uid];
        const projected = await page.request.get(
          base +
            "/inventory?type=configmap&namespace_mode=explicit&namespace_ids=" +
            namespaceId +
            "&name=" +
            name,
        );
        expect(projected.status()).toBe(200);
        expect((await projected.json()).items).toHaveLength(1);
        const violations = (
          await new AxeBuilder({ page }).analyze()
        ).violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        );
        expect(violations).toEqual([]);
        await page.screenshot({
          path: info.outputPath("real-workbench.png"),
          fullPage: true,
        });
        expect(errors).toEqual([]);
      });
    }
  test("actual Pod logs and interactive terminal", async ({ page }, info) => {
    await page.setViewportSize({ width: 1440, height: 1050 });
    await page.addInitScript(() => {
      localStorage.setItem("argus.locale", "zh-CN");
      localStorage.setItem("argus.theme", "light");
    });
    await page.goto(`/kubernetes/${cluster}?tab=resources`);
    await selectWorkbenchNamespace(page, namespace);
    await page
      .getByRole("tablist", { name: "资源类型", exact: true })
      .getByRole("tab", { name: "Pod", exact: true })
      .click();
    const pod = page
      .locator(".argus-k8s-workbench")
      .getByRole("row")
      .filter({ has: page.getByText(/^b10a-api-/) })
      .first();
    await expect(pod).toBeVisible();
    await pod.getByRole("button", { name: "查看日志", exact: true }).click();
    const logs = page.getByRole("dialog", { name: /^Pod 日志/ });
    await expect(logs.getByRole("log")).toContainText("p9-b10a", {
      timeout: 30_000,
    });
    await page.screenshot({
      path: info.outputPath("actual-pod-logs.png"),
      fullPage: true,
    });
    await logs
      .getByRole("button", { name: "关闭日志会话", exact: true })
      .click();
    await expect(logs).not.toBeVisible();
    await pod.getByRole("button", { name: "打开终端", exact: true }).click();
    const terminal = page.getByRole("dialog", { name: /^Pod 终端/ });
    await terminal
      .getByRole("textbox", { name: "操作原因", exact: true })
      .fill("B10a actual terminal output validation");
    const outputs: string[] = [];
    page.on("websocket", (socket) =>
      socket.on("framereceived", (event) => {
        try {
          const frame = JSON.parse(String(event.payload));
          if (frame.type === "output")
            outputs.push(Buffer.from(frame.data, "base64").toString());
        } catch {
          /* binary traffic is not a terminal output frame */
        }
      }),
    );
    await terminal
      .getByRole("button", { name: "创建会话", exact: true })
      .click();
    await expect(terminal.getByText("已连接", { exact: false })).toBeVisible({
      timeout: 35_000,
    });
    await terminal.locator(".xterm-helper-textarea").focus();
    await page.keyboard.type("printf 'B10A_ACTUAL_EXEC'; cat /config/mode");
    await page.keyboard.press("Enter");
    await expect
      .poll(() => outputs.join(""), { timeout: 25_000 })
      .toContain("updated");
    await page.screenshot({
      path: info.outputPath("actual-terminal.png"),
      fullPage: true,
    });
    await terminal
      .getByRole("button", { name: "关闭会话", exact: true })
      .click();
    await expect(terminal).not.toBeVisible();
  });
});
