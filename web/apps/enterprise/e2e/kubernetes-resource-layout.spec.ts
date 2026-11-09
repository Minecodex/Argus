import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`Kubernetes resource layout ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(90_000);
        const text = (zh: string, en: string) => (english ? en : zh);
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.setViewportSize({ width, height: 1000 });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        await page.goto("/login");
        await page.locator('input[autocomplete="username"]').fill("root");
        await page
          .locator('input[autocomplete="current-password"]')
          .fill("123456");
        await page.locator('form button[type="submit"]').click();
        await expect(page).not.toHaveURL(/\/login/);
        await page.goto("/kubernetes/k8s-prod-east");
        const categoryTabs = page.getByRole("tablist", {
          name: text("资源类别", "Resource category"),
          exact: true,
        });
        const typeTabs = page.getByRole("tablist", {
          name: text("资源类型", "Resource type"),
          exact: true,
        });
        await expect(categoryTabs).toBeVisible();
        await expect(
          page.locator(".argus-k8s-workbench").getByRole("switch"),
        ).toHaveCount(0);
        await expect(
          page
            .locator(".argus-k8s-workbench")
            .getByText(/^(实时更新中|实时更新已关闭|Live|Live updates off)$/),
        ).toHaveCount(0);
        await expect(
          typeTabs.getByRole("tab", { name: "Deployment", exact: true }),
        ).toHaveAttribute("aria-selected", "true");
        const bar = page.locator(".argus-k8s-resource-filters");
        const namespace = bar.getByRole("combobox", {
          name: text("命名空间范围", "Namespace scope"),
          exact: true,
        });
        const name = bar.getByRole("searchbox");
        const labels = bar.getByRole("textbox", {
          name: text("标签选择器", "Label selector"),
          exact: true,
        });
        const refresh = bar.getByRole("button", {
          name: text("刷新", "Refresh"),
          exact: true,
        });
        const history = bar.getByRole("button", {
          name: text("操作记录", "Operation history"),
          exact: true,
        });
        const create = bar.getByRole("button", {
          name: text("新建对象", "Create object"),
          exact: true,
        });
        await expect(create).toBeDisabled();
        await expect(namespace).toHaveValue(
          text("全部已授权命名空间", "All authorized namespaces"),
        );
        const boxes = (
          await Promise.all([
            bar.locator(".argus-combobox__control").boundingBox(),
            bar.locator(".argus-search-input").boundingBox(),
            labels.boundingBox(),
            refresh.boundingBox(),
            history.boundingBox(),
            create.boundingBox(),
          ])
        ).map((box) => box!);
        expect(
          Math.max(...boxes.map((box) => box.y)) -
            Math.min(...boxes.map((box) => box.y)),
        ).toBeLessThanOrEqual(1);
        expect(
          Math.max(...boxes.map((box) => box.height)) -
            Math.min(...boxes.map((box) => box.height)),
        ).toBeLessThanOrEqual(1);
        expect(boxes[4]!.x + boxes[4]!.width).toBeLessThan(boxes[5]!.x);
        const frame = (await bar.boundingBox())!;
        expect(
          frame.x + frame.width - boxes[5]!.x - boxes[5]!.width,
        ).toBeLessThanOrEqual(18);
        expect(
          await bar.evaluate((node) => node.scrollWidth <= node.clientWidth),
        ).toBe(true);
        await expect(
          page.getByRole("button", { name: /Helm/, exact: false }),
        ).toHaveCount(0);

        await selectWorkbenchNamespace(page, "production");
        await expect(create).toBeEnabled();
        const deploymentRows = page.getByRole("row").filter({
          has: page.getByText("deployment-example", { exact: true }),
        });
        await expect(deploymentRows).toHaveCount(1);
        await expect(deploymentRows).toContainText("production");
        await create.click();
        await expect(
          page.getByRole("dialog", {
            name: text("新建 Deployment", "Create Deployment"),
            exact: true,
          }),
        ).toBeVisible();
        await page.keyboard.press("Escape");

        await name.fill("missing-k8s-object");
        await name.press("Enter");
        await expect(
          page.getByText(text("当前页没有资源", "No resources on this page"), {
            exact: true,
          }),
        ).toBeVisible();
        await name.fill("deployment-example");
        await labels.fill("app=example");
        await refresh.click();
        await expect(deploymentRows).toHaveCount(1);
        await name.fill("");
        await labels.fill("");
        await name.press("Enter");

        await typeTabs.getByRole("tab", { name: "Pod", exact: true }).click();
        await expect(
          page.getByText("pod-example", { exact: true }),
        ).toBeVisible();
        await expect(
          page.getByRole("button", {
            name: text("端口转发", "Port forward"),
            exact: true,
          }),
        ).toHaveCount(0);
        await page.getByRole("heading", { level: 1 }).click();
        await page.locator(".argus-k8s-workbench").screenshot({
          path: info.outputPath("kubernetes-pod-workspace.png"),
        });
        await categoryTabs
          .getByRole("tab", { name: text("网络", "Networking"), exact: true })
          .click();
        await typeTabs
          .getByRole("tab", { name: "Service", exact: true })
          .click();
        await expect(
          page.getByText("service-example", { exact: true }),
        ).toBeVisible();
        await expect(
          page.getByRole("button", {
            name: text("端口转发", "Port forward"),
            exact: true,
          }),
        ).toHaveCount(0);
        await categoryTabs
          .getByRole("tab", { name: text("集群", "Cluster"), exact: true })
          .click();
        await typeTabs.getByRole("tab", { name: "Node", exact: true }).click();
        await expect(namespace).toHaveCount(0);
        await categoryTabs
          .getByRole("tab", {
            name: text("工作负载", "Workloads"),
            exact: true,
          })
          .click();
        await typeTabs
          .getByRole("tab", { name: "Deployment", exact: true })
          .click();
        await expect(namespace).toHaveValue("production");
        await namespace.fill("");
        await page
          .getByRole("option", {
            name: text("全部已授权命名空间", "All authorized namespaces"),
            exact: true,
          })
          .click();
        await expect(create).toBeDisabled();
        await expect(deploymentRows).toHaveCount(2);
        await history.click();
        await expect(
          page.getByRole("dialog", {
            name: text("操作记录", "Operation history"),
            exact: true,
          }),
        ).toBeVisible();
        await page.keyboard.press("Escape");
        expect(
          (await new AxeBuilder({ page }).analyze()).violations.filter(
            (item) => item.impact === "serious" || item.impact === "critical",
          ),
        ).toEqual([]);
        await page.screenshot({
          path: info.outputPath("kubernetes-resources.png"),
          fullPage: true,
        });
        expect(errors).toEqual([]);
      });
    }
