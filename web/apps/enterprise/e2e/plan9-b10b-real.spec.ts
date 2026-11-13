import { selectRowAction } from "./helpers/action-menu";
import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
const cluster = process.env.ARGUS_PLAN9_B10B_CLUSTER ?? "";
const namespace = "b10b-workloads";
const base = "/api/v1/enterprise/kubernetes-clusters/" + cluster;
test.describe("B10b real storage and jobs", () => {
  test.skip(
    process.env.ARGUS_PLAN9_B10B_E2E !== "1",
    "Requires owned B10b runtime and normal login",
  );
  test.describe.configure({ mode: "serial", timeout: 180_000 });
  test.beforeEach(async ({ context }) => {
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
      test(`actual inventory and CronJob run ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        const text = (zh: string, en: string) => (english ? en : zh);
        const errors: string[] = [];
        page.on("pageerror", (e) => errors.push(e.message));
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
        const cases = [
          ["存储", "Storage", "PersistentVolumeClaim", "b10b-data"],
          ["存储", "Storage", "PersistentVolume", "pvc-"],
          ["存储", "Storage", "StorageClass", "b10b-expandable"],
          [
            "弹性伸缩",
            "Autoscaling",
            "HorizontalPodAutoscaler",
            "b10b-autoscale",
          ],
          ["网络", "Networking", "NetworkPolicy", "b10b-storage-allow-client"],
          ["工作负载", "Workloads", "Job", "b10b-job"],
          ["工作负载", "Workloads", "CronJob", "b10b-cron"],
        ];
        for (const [zh, en, kind, name] of cases) {
          await categories
            .getByRole("tab", { name: text(zh!, en!), exact: true })
            .click();
          await types.getByRole("tab", { name: kind, exact: true }).click();
          await expect(
            workbench
              .getByRole("row")
              .filter({
                has: page.getByText(name === "pvc-" ? /^pvc-/ : name!, {
                  exact: name !== "pvc-",
                }),
              })
              .first(),
          ).toBeVisible();
          await page.screenshot({
            path: info.outputPath(kind + ".png"),
            fullPage: true,
          });
        }
        const cron = workbench
          .getByRole("row")
          .filter({ has: page.getByText("b10b-cron", { exact: true }) });
        await cron
          .getByRole("button", {
            name: text("运行一次", "Run once"),
            exact: true,
          })
          .click();
        const dialog = page.getByRole("dialog", {
          name: text(
            "从 b10b-cron 创建一次 Job",
            "Create a Job from b10b-cron",
          ),
          exact: true,
        });
        await expect(dialog).toBeVisible();
        const name = `b10b-ui-${english ? "en" : "zh"}-${theme}-${Date.now().toString().slice(-6)}`;
        await dialog
          .getByRole("textbox", {
            name: text("Job 名称", "Job name"),
            exact: true,
          })
          .fill(name);
        await dialog
          .getByRole("button", {
            name: text("验证并预览差异", "Validate and preview"),
            exact: true,
          })
          .click();
        await expect(dialog).toContainText(name);
        await page.screenshot({
          path: info.outputPath("manual-job-preview.png"),
          fullPage: true,
        });
        await dialog
          .getByRole("button", {
            name: text("确认执行", "Confirm operation"),
            exact: true,
          })
          .click();
        await expect
          .poll(
            async () => {
              const response = await page.request.get(
                base + "/operations?limit=200",
              );
              expect(response.status()).toBe(200);
              return (await response.json()).items.some(
                (row: { name: string; status: string }) =>
                  row.name === name && row.status === "succeeded",
              );
            },
            { timeout: 90_000 },
          )
          .toBe(true);
        const history = page.getByRole("dialog", {
          name: text("操作记录", "Operation history"),
          exact: true,
        });
        await history
          .getByRole("button", { name: text("关闭", "Close"), exact: true })
          .click();
        await expect(history).not.toBeVisible();
        await types.getByRole("tab", { name: "Job", exact: true }).click();
        await workbench
          .getByRole("searchbox", {
            name: text("按资源完整名称查找", "Find by exact resource name"),
            exact: true,
          })
          .fill(name);
        await workbench
          .getByRole("button", { name: text("刷新", "Refresh"), exact: true })
          .click();
        await expect(
          workbench
            .getByRole("row")
            .filter({ has: page.getByText(name, { exact: true }) }),
        ).toBeVisible();
        const violations = (
          await new AxeBuilder({ page }).analyze()
        ).violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        );
        expect(violations).toEqual([]);
        await page.goto(`/kubernetes/${cluster}?tab=resources`);
        await page
          .getByRole("tab", {
            name: text("采集配置", "Collection"),
            exact: true,
          })
          .click();
        await expect(
          page.getByRole("tab", {
            name: text("采集配置", "Collection"),
            exact: true,
          }),
        ).toHaveAttribute("aria-selected", "true");
        const nodeRow = page.getByRole("row").filter({
          has: page.getByText(
            text("节点与容器指标", "Node and container metrics"),
            { exact: true },
          ),
        });
        await nodeRow
          .getByRole("button", {
            name: text(
              "节点与容器指标 · 更多操作",
              "Node and container metrics · More actions",
            ),
            exact: true,
          })
          .click();
        await page
          .getByRole("menuitem", {
            name: text("证书信任", "Certificate trust"),
            exact: true,
          })
          .click();
        const trust = page.getByRole("dialog", {
          name: text("组件指标证书信任", "Component metric certificate trust"),
          exact: true,
        });
        await expect(
          trust.getByRole("textbox", {
            name: text(
              "受信任的公开证书（PEM）",
              "Trusted public certificates (PEM)",
            ),
            exact: true,
          }),
        ).toHaveValue(/BEGIN CERTIFICATE/);
        await expect(
          trust.getByRole("textbox", {
            name: text("证书服务名", "Certificate server name"),
            exact: true,
          }),
        ).toHaveCount(0);
        await page.evaluate(async () => {
          const finite = document
            .getAnimations()
            .filter((animation) =>
              Number.isFinite(animation.effect?.getComputedTiming().endTime),
            );
          await Promise.all(
            finite.map((animation) =>
              animation.finished.catch(() => undefined),
            ),
          );
        });
        expect(
          (await new AxeBuilder({ page }).analyze()).violations.filter(
            (v) => v.impact === "critical" || v.impact === "serious",
          ),
        ).toEqual([]);
        await page.screenshot({
          path: info.outputPath("native-node-public-ca.png"),
          fullPage: true,
        });
        await trust
          .getByRole("button", { name: text("取消", "Cancel"), exact: true })
          .click();
        expect(errors).toEqual([]);
        await page.screenshot({
          path: info.outputPath("completed-browser-job.png"),
          fullPage: true,
        });
      });
    }
  test("actual completed Job logs and persistent storage terminal", async ({
    page,
  }, info) => {
    await page.setViewportSize({ width: 1440, height: 1050 });
    await page.addInitScript(() => {
      localStorage.setItem("argus.locale", "zh-CN");
      localStorage.setItem("argus.theme", "light");
    });
    await page.goto(`/kubernetes/${cluster}?tab=resources`);
    await selectWorkbenchNamespace(page, namespace);
    const types = page.getByRole("tablist", { name: "资源类型", exact: true });
    await types.getByRole("tab", { name: "Pod", exact: true }).click();
    const workbench = page.locator(".argus-k8s-workbench");
    const job = workbench
      .getByRole("row")
      .filter({ has: page.getByText(/^b10b-job-/) })
      .first();
    await selectRowAction(job, "查看日志");
    const logs = page.getByRole("dialog", { name: /^Pod 日志/ });
    await expect(logs.getByRole("log")).toContainText("job-completed", {
      timeout: 30_000,
    });
    await page.screenshot({
      path: info.outputPath("actual-job-logs.png"),
      fullPage: true,
    });
    await logs
      .getByRole("button", { name: "关闭日志会话", exact: true })
      .click();
    const storage = workbench
      .getByRole("row")
      .filter({ has: page.getByText(/^b10b-storage-/) })
      .first();
    await selectRowAction(storage, "打开终端");
    const terminal = page.getByRole("dialog", { name: /^Pod 终端/ });
    await terminal
      .getByRole("textbox", { name: "操作原因", exact: true })
      .fill("B10b verify actual mounted filesystem");
    const output: string[] = [];
    page.on("websocket", (socket) =>
      socket.on("framereceived", (event) => {
        try {
          const frame = JSON.parse(String(event.payload));
          if (frame.type === "output")
            output.push(Buffer.from(frame.data, "base64").toString());
        } catch {
          /* Non-JSON socket frames are not terminal output. */
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
    await page.keyboard.type(
      "printf 'B10B_STORAGE_OK'; cat /data/receipt.json; df -k /data",
    );
    await page.keyboard.press("Enter");
    await expect
      .poll(() => output.join(""), { timeout: 25_000 })
      .toContain("bounded-background");
    await page.screenshot({
      path: info.outputPath("actual-storage-terminal.png"),
      fullPage: true,
    });
    await terminal
      .getByRole("button", { name: "关闭会话", exact: true })
      .click();
    await expect(terminal).not.toBeVisible();
  });
});
