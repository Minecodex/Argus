import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";
import { selectRowAction } from "./helpers/action-menu";
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
const root = process.env.ARGUS_KWR_ARTIFACTS ?? "";
const mapping = root
  ? JSON.parse(readFileSync(root + "/lab-resources.json", "utf8"))
  : {};
test.describe("KWR actual resource operations", () => {
  test.skip(
    process.env.ARGUS_KWR_REAL !== "1",
    "Requires owned actual cluster",
  );
  test("no Collector, actual scale, configuration, Job, logs and terminal", async ({
    page,
  }, info) => {
    test.setTimeout(240000);
    await page.setViewportSize({ width: 1440, height: 1100 });
    await page.addInitScript(() => {
      localStorage.setItem("argus.locale", "zh-CN");
      localStorage.setItem("argus.theme", "light");
    });
    await page.goto("/login");
    await page.locator('input[autocomplete="username"]').fill("kakj");
    await page
      .locator('input[autocomplete="current-password"]')
      .fill(process.env.ARGUS_KWR_PASSWORD!);
    await page.locator('form button[type="submit"]').click();
    await expect(page).not.toHaveURL(/login/);
    const executed = page.waitForResponse(
      (r) =>
        r.url().endsWith("/overview-dashboard/execute") && r.status() === 200,
    );
    await page.goto("/kubernetes/" + mapping.cluster);
    const snapshot = await (await executed).json();
    expect(
      snapshot.panels.find((p: { id: string }) => p.id === "health").targets[0]
        .data.counts.node.total,
    ).toBe(2);
    for (const id of ["cpu", "memory", "cpu_requests", "memory_requests"])
      expect(
        snapshot.panels.find((p: { id: string }) => p.id === id).status,
      ).toBe("no_data");
    await page.screenshot({
      path: info.outputPath("no-collector-overview.png"),
      fullPage: true,
    });
    await page.getByRole("tab", { name: "资源查询", exact: true }).click();
    await selectWorkbenchNamespace(page, "kwr-workloads");
    const row = page.getByRole("row").filter({
      has: page.getByRole("button", { name: "kwr-api", exact: true }),
    });
    await selectRowAction(row, "扩缩容");
    let dialog = page.getByRole("dialog", {
      name: "扩缩容 · kwr-api",
      exact: true,
    });
    await dialog.getByRole("spinbutton", { name: /副本/ }).fill("2");
    await dialog
      .getByRole("button", { name: "验证并预览差异", exact: true })
      .click();
    await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(dialog).not.toBeVisible({ timeout: 5000 });
    await expect(page.locator(".argus-k8s-operation-feedback")).toContainText(
      "已生效",
      { timeout: 90000 },
    );
    await expect(row).toContainText("2 / 2");
    await page.getByRole("tab", { name: "配置", exact: true }).click();
    await page.getByRole("tab", { name: "ConfigMap", exact: true }).click();
    const config = page.getByRole("row").filter({
      has: page.getByRole("button", { name: "kwr-config", exact: true }),
    });
    await selectRowAction(config, "编辑配置");
    dialog = page.getByRole("dialog", {
      name: "编辑配置 · kwr-config",
      exact: true,
    });
    await dialog
      .getByRole("group", { name: "配置数据", exact: true })
      .getByRole("textbox", { name: "值 1", exact: true })
      .fill("edited-through-real-ui");
    await dialog
      .getByRole("button", { name: "验证并预览差异", exact: true })
      .click();
    await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(page.locator(".argus-k8s-operation-feedback")).toContainText(
      "已生效",
      { timeout: 45000 },
    );
    await page.getByRole("tab", { name: "工作负载", exact: true }).click();
    await page.getByRole("tab", { name: "CronJob", exact: true }).click();
    const cron = page.getByRole("row").filter({
      has: page.getByRole("button", { name: "kwr-cron", exact: true }),
    });
    await selectRowAction(cron, "立即运行");
    dialog = page.getByRole("dialog", {
      name: "从 kwr-cron 创建一次 Job",
      exact: true,
    });
    await dialog
      .getByRole("textbox", { name: /Job 名称/ })
      .fill("kwr-browser-job");
    await dialog
      .getByRole("button", { name: "验证并预览差异", exact: true })
      .click();
    await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(page.locator(".argus-k8s-operation-feedback")).toContainText(
      "已生效",
      { timeout: 60000 },
    );
    await page.getByRole("tab", { name: "Pod", exact: true }).click();
    const pod = page
      .getByRole("row")
      .filter({ has: page.getByRole("button", { name: /^kwr-api-/ }) })
      .first();
    await selectRowAction(pod, "查看日志");
    dialog = page.getByRole("dialog", { name: /^Pod 日志/ });
    await expect(dialog.getByText(/kwr-actual-log/).first()).toBeVisible({
      timeout: 30000,
    });
    await dialog
      .getByRole("button", { name: "展开排障工作区", exact: true })
      .click();
    await expect(dialog).toHaveClass(/argus-dialog--cover/);
    await page.screenshot({
      path: info.outputPath("actual-logs.png"),
      fullPage: true,
    });
    await dialog
      .getByRole("button", { name: "关闭日志会话", exact: true })
      .click();
    const frames: string[] = [];
    page.on("websocket", (socket) =>
      socket.on("framereceived", (event) => {
        try {
          const frame = JSON.parse(String(event.payload));
          if (frame.type === "output")
            frames.push(Buffer.from(frame.data, "base64").toString());
        } catch {
          /* Other frames are not terminal output. */
        }
      }),
    );
    await selectRowAction(pod, "打开终端");
    dialog = page.getByRole("dialog", { name: /^Pod 终端/ });
    await dialog
      .getByRole("textbox", { name: /操作原因/ })
      .fill("KWR verify actual owned shell");
    await dialog.getByRole("button", { name: "创建会话", exact: true }).click();
    await expect(dialog.getByText("已连接", { exact: false })).toBeVisible({
      timeout: 30000,
    });
    await dialog.locator(".xterm-helper-textarea").focus();
    await page.keyboard.type("printf 'KWR_TERMINAL_OK\\n'; id; pwd");
    await page.keyboard.press("Enter");
    await expect
      .poll(() => frames.join(""), { timeout: 30000 })
      .toContain("KWR_TERMINAL_OK");
    await expect.poll(() => frames.join("")).toMatch(/uid=\d+/);
    await page.screenshot({
      path: info.outputPath("actual-terminal.png"),
      fullPage: true,
    });
    await dialog.getByRole("button", { name: "关闭会话", exact: true }).click();
  });
  test("namespace-only normal login cannot see node capacity or management actions", async ({
    page,
  }, info) => {
    test.setTimeout(90000);
    const login = JSON.parse(
      readFileSync(root + "/private/reader-login.json", "utf8"),
    );
    await page.goto("/login");
    await page.locator('input[autocomplete="username"]').fill(login.username);
    await page
      .locator('input[autocomplete="current-password"]')
      .fill(login.password);
    await page.locator('form button[type="submit"]').click();
    await expect(page).not.toHaveURL(/login/);
    const executed = page.waitForResponse(
      (r) =>
        r.url().endsWith("/overview-dashboard/execute") && r.status() === 200,
    );
    await page.goto("/kubernetes/" + mapping.cluster);
    const data = await (await executed).json();
    expect(data.resources).toEqual([
      { type: "kubernetes_namespace", id: mapping.namespace },
    ]);
    const native = data.panels.find((p: { id: string }) => p.id === "nodes")
      .targets[0].data;
    expect(native.cluster_wide).toBe(false);
    expect(
      native.resources.every(
        (r: { type: string; namespace: string }) =>
          r.type !== "node" && r.namespace === "kwr-workloads",
      ),
    ).toBe(true);
    await expect(
      page.getByRole("button", { name: "选择大盘", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.locator(".argus-node-allocation").getByText(/可分配/),
    ).toHaveCount(0);
    await page.getByRole("tab", { name: "资源查询", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "kwr-api", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "新建对象", exact: true }),
    ).toHaveCount(0);
    await expect(page.getByRole("button", { name: /更多操作/ })).toHaveCount(0);
    await page.screenshot({
      path: info.outputPath("restricted-namespace.png"),
      fullPage: true,
    });
  });
});
