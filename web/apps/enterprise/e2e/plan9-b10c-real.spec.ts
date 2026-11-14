import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { selectRowAction } from "./helpers/action-menu";

const enabled = process.env.ARGUS_PLAN9_B10C_E2E === "1";
const mapping = enabled
  ? JSON.parse(readFileSync(process.env.ARGUS_PLAN9_B10C_MAP!, "utf8"))
  : {};
test.use({
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
  ignoreHTTPSErrors: false,
});
test.skip(
  !enabled,
  "Requires the owned B10c runtime, real backend and normal login",
);

async function login(
  page: import("@playwright/test").Page,
  english = false,
  theme = "light",
) {
  page.setDefaultTimeout(20000);
  await page.setViewportSize({
    width: theme === "light" ? 1280 : 1920,
    height: 1080,
  });
  await page.addInitScript(
    ({ english, theme }) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("kakj");
  await page
    .locator('input[autocomplete="current-password"]')
    .fill(process.env.ARGUS_PLAN9_PASSWORD!);
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`B10c actual overview, resources and alert history ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(180000);
      const text = (zh: string, en: string) => (english ? en : zh);
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await login(page, english, theme);
      const cluster = mapping.cluster.id,
        namespace = mapping.namespaces["b10c-workloads"].id;
      const path = `/kubernetes/${cluster}`;
      await page.goto(path);
      await expect(
        page.locator(".argus-resource-summary__item").first(),
      ).toContainText("3 / 3", { timeout: 45000 });
      await page.screenshot({
        path: info.outputPath("actual-overview.png"),
        animations: "disabled",
      });
      await page.goto(
        path +
          `?tab=resources&kind=deployment&namespace_id=${namespace}&namespace_name=b10c-workloads`,
      );
      for (const width of [1280, 1440, 1920]) {
        await page.setViewportSize({ width, height: 1080 });
        const inventory = page.locator(".argus-k8s-inventory-table");
        const releaseRow = inventory.locator("tbody > tr").filter({
          has: page.getByRole("button", { name: "b10c-repo", exact: true }),
        });
        await expect(releaseRow).toBeVisible({ timeout: 45000 });
        const headerColors = await inventory
          .locator("th")
          .evaluateAll((cells) =>
            cells.map((cell) => getComputedStyle(cell).backgroundColor),
          );
        expect(new Set(headerColors).size).toBe(1);
        expect(headerColors[0]).not.toBe("rgba(0, 0, 0, 0)");
        await page.mouse.move(16, 300);
        const normalColors = await releaseRow.evaluate((row) => ({
          row: getComputedStyle(row).backgroundColor,
          action: getComputedStyle(row.lastElementChild!).backgroundColor,
        }));
        expect(normalColors.action).toBe(normalColors.row);
        // Inspect the plain namespace cell; the name now opens its own summary.
        await releaseRow.locator("td").nth(1).hover();
        const hoverColors = await releaseRow.evaluate((row) => ({
          row: getComputedStyle(row).backgroundColor,
          action: getComputedStyle(row.lastElementChild!).backgroundColor,
        }));
        expect(hoverColors.action).toBe(hoverColors.row);
        expect(hoverColors.row).not.toBe(normalColors.row);
        await page.screenshot({
          path: info.outputPath(`actual-table-action-hover-${width}.png`),
          animations: "disabled",
        });
        await page.mouse.move(16, 300);
        const inventoryWrap = page.locator(".argus-k8s-inventory-wrap");
        await inventoryWrap.evaluate((element) => {
          element.scrollLeft = element.scrollWidth;
        });
        const actionHeader = inventory.locator("th").last();
        expect(
          await actionHeader.evaluate(
            (cell) => getComputedStyle(cell).backgroundColor,
          ),
        ).toBe(headerColors[0]);
        const actionBounds = (await actionHeader.boundingBox())!;
        const wrapBounds = (await inventoryWrap.boundingBox())!;
        expect(actionBounds.x + actionBounds.width).toBeLessThanOrEqual(
          wrapBounds.x + wrapBounds.width + 1,
        );
        await page.screenshot({
          path: info.outputPath(`actual-table-action-header-${width}.png`),
          animations: "disabled",
        });
        await inventoryWrap.evaluate((element) => {
          element.scrollLeft = 0;
        });
      }
      await page.setViewportSize({
        width: theme === "light" ? 1280 : 1920,
        height: 1080,
      });
      await page
        .getByRole("button", { name: "b10c-repo", exact: true })
        .first()
        .click();
      await expect(
        page.getByRole("dialog", { name: "b10c-repo", exact: true }),
      ).toHaveCount(0);
      let drawer = page.getByRole("dialog", {
        name: "Deployment · b10c-repo",
        exact: true,
      });
      await expect(drawer).toContainText("b10c-workloads");
      await drawer
        .getByRole("button", {
          name: text("放大详情窗口", "Expand detail window"),
          exact: true,
        })
        .click();
      await page.screenshot({
        path: info.outputPath("actual-helm-workload.png"),
        animations: "disabled",
      });
      await page.mouse.click(16, 300);
      await expect(drawer).toHaveCount(0);
      await expect(
        page.getByRole("dialog", { name: "b10c-repo", exact: true }),
      ).toHaveCount(0);
      await page.goto(path + "?tab=resources&kind=node");
      await page
        .getByRole("button", { name: mapping.maintenance_node, exact: true })
        .first()
        .click();
      await expect(
        page.getByRole("dialog", {
          name: mapping.maintenance_node,
          exact: true,
        }),
      ).toHaveCount(0);
      drawer = page.getByRole("dialog");
      await drawer
        .getByRole("tab", { name: text("配置", "Configuration"), exact: true })
        .click();
      await expect(drawer).toContainText("verified");
      await page.screenshot({
        path: info.outputPath("actual-maintenance-node.png"),
        animations: "disabled",
      });
      const deniedRequest = page.waitForResponse((response) => {
        const url = new URL(response.url());
        return (
          url.pathname.endsWith("/inventory") &&
          url.searchParams.get("type") === mapping.custom_resource.type &&
          url.searchParams.get("namespace_mode") === "all_authorized"
        );
      });
      await page.goto(
        path +
          `?tab=resources&kind=${encodeURIComponent(mapping.custom_resource.type)}&name_filter=b10c-check`,
      );
      expect((await deniedRequest).status()).toBe(403);
      const denied = page.locator(".argus-k8s-inventory-error");
      await expect(denied).toContainText(
        text(
          "当前用户或集群接入身份无权访问该范围。",
          "The current user or cluster connection identity cannot access this scope.",
        ),
      );
      await expect(denied).not.toContainText(
        text(
          "暂时无法读取集群资源。",
          "Cluster resources are temporarily unavailable.",
        ),
      );
      await page.screenshot({
        path: info.outputPath("actual-namespace-scope-denied.png"),
        animations: "disabled",
      });
      await denied
        .getByRole("button", {
          name: text("选择命名空间", "Choose a namespace"),
          exact: true,
        })
        .click();
      const picker = page.getByRole("dialog", {
        name: text("选择命名空间", "Choose a namespace"),
        exact: true,
      });
      await picker
        .getByRole("button", {
          name: text("选择 b10c-workloads", "Select b10c-workloads"),
          exact: true,
        })
        .click();
      await expect(picker).toHaveCount(0);
      await expect(denied).toHaveCount(0);
      expect(new URL(page.url()).searchParams.get("namespace_id")).toBe(
        namespace,
      );
      expect(new URL(page.url()).searchParams.get("name_filter")).toBe(
        "b10c-check",
      );
      await expect(
        page.getByRole("searchbox", {
          name: text("按资源完整名称查找", "Find by exact resource name"),
        }),
      ).toHaveValue("b10c-check");
      await page
        .getByRole("button", { name: "b10c-check", exact: true })
        .first()
        .click();
      await expect(
        page.getByRole("dialog", { name: "b10c-check", exact: true }),
      ).toHaveCount(0);
      drawer = page.getByRole("dialog");
      await drawer
        .getByRole("tab", { name: "YAML / JSON", exact: true })
        .click();
      await expect(drawer).toContainText(
        "Updated through the normal Argus workbench",
      );
      await page.screenshot({
        path: info.outputPath("actual-custom-resource.png"),
        animations: "disabled",
      });
      await page.goto(
        path +
          `?tab=resources&kind=role&namespace_id=${namespace}&namespace_name=b10c-workloads`,
      );
      await page
        .getByRole("button", { name: "b10c-observer", exact: true })
        .first()
        .click();
      await expect(
        page.getByRole("dialog", { name: "b10c-observer", exact: true }),
      ).toHaveCount(0);
      drawer = page.getByRole("dialog");
      await drawer
        .getByRole("tab", { name: text("配置", "Configuration"), exact: true })
        .click();
      await expect(drawer).toContainText("pods");
      await expect(drawer).toContainText("list");
      await page.goto(path + "?tab=dashboards");
      await expect(page.getByText(/B10c Helm/).first()).toBeVisible();
      await page.screenshot({
        path: info.outputPath("actual-dashboard-bindings.png"),
        animations: "disabled",
      });
      await page.goto("/alerts");
      await page
        .getByRole("tab", {
          name: text("告警历史", "Alert history"),
          exact: true,
        })
        .click();
      await expect(
        page
          .getByText("B10c released workload has no available replica", {
            exact: true,
          })
          .first(),
      ).toBeVisible({ timeout: 30000 });
      await page.screenshot({
        path: info.outputPath("actual-alert-history.png"),
        animations: "disabled",
      });
      expect(errors).toEqual([]);
    });
  }

test("B10c actual Pod log and terminal output", async ({ page }, info) => {
  test.setTimeout(120000);
  await login(page);
  await page.goto(
    `/kubernetes/${mapping.cluster.id}?tab=resources&kind=pod&namespace_id=${mapping.namespaces["b10c-workloads"].id}&namespace_name=b10c-workloads`,
  );
  const pod = page
    .locator(".argus-k8s-workbench tbody > tr")
    .filter({ has: page.getByRole("button", { name: /^b10c-repo-/ }) })
    .first();
  await expect(pod).toBeVisible({ timeout: 45000 });
  await selectRowAction(pod, "查看日志");
  const logs = page.getByRole("dialog", { name: /^Pod 日志/ });
  await expect(logs.getByRole("log")).toContainText("http-request", {
    timeout: 30000,
  });
  await logs.getByRole("button", { name: "关闭日志会话", exact: true }).click();
  await selectRowAction(pod, "打开终端");
  const terminal = page.getByRole("dialog", { name: /^Pod 终端/ });
  await terminal
    .getByRole("textbox", { name: "操作原因", exact: true })
    .fill("B10c verify real release container identity");
  const output: string[] = [];
  page.on("websocket", (socket) =>
    socket.on("framereceived", (frame) => {
      try {
        const value = JSON.parse(String(frame.payload));
        if (value.type === "output")
          output.push(Buffer.from(value.data, "base64").toString());
      } catch {
        /* Ignore non-JSON frames. */
      }
    }),
  );
  await terminal.getByRole("button", { name: "创建会话", exact: true }).click();
  await expect(terminal.getByText("已连接", { exact: false })).toBeVisible({
    timeout: 35000,
  });
  await terminal.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type(
    'python3 -c \'import os,json; print(json.dumps({"run":os.environ["PLAN9_RUN"],"pid":os.getpid()}))\'',
  );
  await page.keyboard.press("Enter");
  await expect
    .poll(() => output.join(""), { timeout: 20000 })
    .toContain('"run": "p9-b10c-20261113-01"');
  await page.screenshot({
    path: info.outputPath("actual-release-terminal.png"),
    animations: "disabled",
  });
  await terminal.getByRole("button", { name: "关闭会话", exact: true }).click();
  await expect(terminal).toHaveCount(0);
});
