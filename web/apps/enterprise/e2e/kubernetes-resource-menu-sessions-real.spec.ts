import { expect, test } from "@playwright/test";
import { selectRowAction } from "./helpers/action-menu";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";
import { dashboardAPI } from "./helpers/planv2-api";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
test("deployed row menus open actual B10b logs and business Pod terminal", async ({
  page,
}, info) => {
  test.skip(
    process.env.ARGUS_KWR_REAL !== "1",
    "Requires existing B10b resources and normal login",
  );
  test.setTimeout(120000);
  page.setDefaultTimeout(20000);
  await page.setViewportSize({ width: 1440, height: 1080 });
  await page.addInitScript(() => localStorage.setItem("argus.locale", "zh-CN"));
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("kakj");
  await page
    .locator('input[autocomplete="current-password"]')
    .fill(process.env.ARGUS_KWR_PASSWORD!);
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
  try {
    await page.goto(
      `/kubernetes/${process.env.ARGUS_KWR_CLUSTER}?tab=resources&kind=pod`,
    );
    await selectWorkbenchNamespace(page, "b10b-workloads");
    const rows = page.locator(".argus-k8s-workbench tbody > tr");
    const job = rows.filter({ has: page.getByText(/^b10b-job-/) }).first();
    await expect(job).toBeVisible({ timeout: 45000 });
    await selectRowAction(job, "查看日志");
    const logs = page.getByRole("dialog", { name: /^Pod 日志/ });
    await expect(logs.getByRole("log")).toContainText("job-completed", {
      timeout: 30000,
    });
    await page.screenshot({
      path: info.outputPath("actual-job-menu-logs.png"),
      animations: "disabled",
    });
    await logs
      .getByRole("button", { name: "关闭日志会话", exact: true })
      .click();
    const storage = rows
      .filter({ has: page.getByText(/^b10b-storage-/) })
      .first();
    await selectRowAction(storage, "打开终端");
    const terminal = page.getByRole("dialog", { name: /^Pod 终端/ });
    await terminal
      .getByRole("textbox", { name: "操作原因", exact: true })
      .fill("Verify resource menu opens existing business Pod terminal");
    const output: string[] = [];
    page.on("websocket", (socket) =>
      socket.on("framereceived", (event) => {
        try {
          const frame = JSON.parse(String(event.payload));
          if (frame.type === "output")
            output.push(Buffer.from(frame.data, "base64").toString());
        } catch {
          /* Ignore non-JSON frames. */
        }
      }),
    );
    await terminal
      .getByRole("button", { name: "创建会话", exact: true })
      .click();
    await expect(terminal.getByText("已连接", { exact: false })).toBeVisible({
      timeout: 35000,
    });
    await terminal.locator(".xterm-helper-textarea").focus();
    await page.keyboard.type(
      "printf 'ARGUS_MENU_TERMINAL_OK\\n'; cat /data/receipt.json; df -k /data",
    );
    await page.keyboard.press("Enter");
    await expect
      .poll(() => output.join(""), { timeout: 25000 })
      .toContain("bounded-background");
    await page.screenshot({
      path: info.outputPath("actual-business-menu-terminal.png"),
      animations: "disabled",
    });
    await terminal
      .getByRole("button", { name: "关闭会话", exact: true })
      .click();
    await expect(terminal).toHaveCount(0);
  } finally {
    await dashboardAPI(
      page,
      "/enterprise/auth/session",
      "DELETE",
      undefined,
      204,
    );
  }
});
