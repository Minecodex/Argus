import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 own metric instances ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      await page.setViewportSize({ width: 1440, height: 1100 });
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.goto("/login");
      await page.locator('input[autocomplete="username"]').fill("root");
      await page
        .locator('input[autocomplete="current-password"]')
        .fill("123456");
      await page.locator('form button[type="submit"]').click();
      await expect(page).not.toHaveURL(/\/login/);
      await page.goto("/hosts/host-web-11");
      await page
        .getByRole("tab", {
          name: english ? "Components & Collection" : "组件与采集",
          exact: true,
        })
        .click();
      const host = page.getByRole("row").filter({
        has: page.getByText(english ? "Host metrics" : "主机基础指标", {
          exact: true,
        }),
      });
      const self = page.getByRole("row").filter({
        has: page.getByText(
          english ? "Collector metrics" : "Collector 自身指标",
          { exact: true },
        ),
      });
      const confirm = async () => {
        const dialog = page.getByRole("dialog", {
          name: english
            ? "Preview own resource monitoring"
            : "自身监控配置预览",
          exact: true,
        });
        await expect(dialog).toBeVisible();
        await dialog.getByRole("button", { name: /^(确认|Confirm)/ }).click();
        await expect(dialog).not.toBeVisible();
      };
      await host.getByRole("switch").focus();
      await host.getByRole("switch").press("Space");
      await confirm();
      await expect(host.getByRole("switch")).toBeChecked();
      await self.getByRole("switch").focus();
      await self.getByRole("switch").press("Space");
      await confirm();
      await expect(self.getByRole("switch")).toBeChecked();
      await host.getByRole("switch").focus();
      await host.getByRole("switch").press("Space");
      await confirm();
      await expect(host.getByRole("switch")).not.toBeChecked();
      await expect(self.getByRole("switch")).toBeChecked();
      await page.screenshot({
        path: info.outputPath("host-own-instances.png"),
        fullPage: true,
      });
      await host
        .getByRole("button", {
          name: english ? "Default dashboard" : "默认仪表盘",
          exact: true,
        })
        .click();
      await expect(page).toHaveURL(/\/dashboards\/[^?]+\?resource=/);
      await expect(
        page
          .getByText(english ? "CPU utilization" : "CPU 使用率", {
            exact: true,
          })
          .first(),
      ).toBeVisible();
      await expect(
        page.getByRole("button", {
          name: english ? "Create personal copy" : "创建个人副本",
          exact: true,
        }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("host-default-dashboard.png"),
        fullPage: true,
      });
      await page.goto("/kubernetes/k8s-prod-east#otlp-collector");
      const nodes = page.getByRole("row").filter({
        has: page.getByText(
          english ? "Node and container metrics" : "节点与容器指标",
          { exact: true },
        ),
      });
      await expect(nodes).toBeVisible();
      await nodes.getByRole("switch").focus();
      await nodes.getByRole("switch").press("Space");
      await confirm();
      await expect(nodes.getByRole("switch")).toBeChecked();
      const a11y = await new AxeBuilder({ page }).analyze();
      expect(
        a11y.violations.filter(
          (item) => item.impact === "serious" || item.impact === "critical",
        ),
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath("kubernetes-own-instances.png"),
        fullPage: true,
      });
      await nodes
        .getByRole("button", {
          name: english ? "Default dashboard" : "默认仪表盘",
          exact: true,
        })
        .click();
      await expect(page).toHaveURL(/resource_type=kubernetes_cluster/);
      await expect(
        page
          .getByText(english ? "Node CPU usage" : "节点 CPU 使用量", {
            exact: true,
          })
          .first(),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("kubernetes-default-dashboard.png"),
        fullPage: true,
      });
      await page.goto("/kubernetes/k8s-prod-east#otlp-collector");
      const apiServer = page.getByRole("row").filter({
        has: page.getByText(
          english ? "API server metrics" : "API Server 指标",
          { exact: true },
        ),
      });
      await apiServer.getByRole("switch").focus();
      await apiServer.getByRole("switch").press("Space");
      await confirm();
      await expect(apiServer.getByRole("switch")).toBeChecked();
      await apiServer
        .getByRole("button", {
          name: english ? "Default dashboard" : "默认仪表盘",
          exact: true,
        })
        .click();
      await expect(
        page
          .getByText(
            english ? "API request duration P95" : "API 请求耗时 P95",
            { exact: true },
          )
          .first(),
      ).toBeVisible();
      await page.goto("/kubernetes/k8s-prod-east#otlp-collector");
      const dns = page
        .getByRole("row")
        .filter({
          has: page.getByText(english ? "CoreDNS metrics" : "CoreDNS 指标", {
            exact: true,
          }),
        });
      await dns.getByRole("switch").focus();
      await dns.getByRole("switch").press("Space");
      await confirm();
      await dns
        .getByRole("button", {
          name: english ? "Default dashboard" : "默认仪表盘",
          exact: true,
        })
        .click();
      await expect(
        page
          .getByText(
            english ? "DNS request duration P95" : "DNS 请求耗时 P95",
            { exact: true },
          )
          .first(),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("kubernetes-coredns-default-dashboard.png"),
        fullPage: true,
      });
      expect(errors).toEqual([]);
    });
  }
