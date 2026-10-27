import { expect, test } from "@playwright/test";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 DNS ICMP Kafka defaults ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(90_000);
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.setViewportSize({ width: 1440, height: 1000 });
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
      for (const target of [
        { type: "DNS", endpoint: "dns.test:53", panel: "status" },
        { type: "ICMP", endpoint: "192.0.2.42", panel: "loss" },
        { type: "Kafka", endpoint: "kafka.test:9092", panel: "consumer_lag" },
      ]) {
        await page.goto("/monitoring");
        await page
          .getByRole("button", {
            name: english ? "New monitor" : "新建监控",
            exact: true,
          })
          .click();
        const types = page.getByRole("dialog", {
          name: english ? "Choose a monitoring type" : "选择监控类型",
        });
        await types
          .getByRole("row")
          .filter({ has: page.getByText(target.type, { exact: true }) })
          .getByRole("button")
          .click();
        const form = page.getByRole("dialog", {
          name: english
            ? `New ${target.type} monitor`
            : `新建 ${target.type} 监控`,
          exact: true,
        });
        await form
          .getByRole("textbox", {
            name: new RegExp(english ? "Monitor name" : "监控名称"),
          })
          .fill(`Owned ${target.type}`);
        await form
          .getByRole("textbox", {
            name: new RegExp(english ? "Service endpoint" : "服务地址"),
          })
          .fill(target.endpoint);
        if (target.type === "DNS") {
          await form
            .getByRole("textbox", {
              name: new RegExp(english ? "Names to resolve" : "待解析域名"),
            })
            .fill("reachable.test");
        }
        await form
          .getByRole("button", {
            name: english ? "Choose a Collector" : "选择执行收集器",
            exact: true,
          })
          .click();
        await page
          .getByRole("dialog", {
            name: english ? "Choose a Collector" : "选择执行收集器",
            exact: true,
          })
          .getByRole("button", { name: /^(选择|Select) / })
          .first()
          .click();
        await form
          .getByRole("button", {
            name: english ? "Preview configuration" : "预览采集配置",
            exact: true,
          })
          .click();
        await page
          .getByRole("button", {
            name: english ? "Confirm changes" : "确认变更",
            exact: true,
          })
          .click();
        const row = page
          .getByRole("row")
          .filter({
            has: page.getByText(`Owned ${target.type}`, { exact: true }),
          });
        await expect(row).toContainText(
          english ? "Waiting for samples" : "等待样本",
        );
        await row
          .getByRole("button", {
            name: english ? "Default dashboard" : "默认仪表盘",
            exact: true,
          })
          .click();
        await expect(page).toHaveURL(/\/dashboards\/[^?]+\?resource=/);
        await expect(
          page.getByRole("button", {
            name: english
              ? "Choose resources: Selected resources: 1"
              : "选择资源: 已选 1 个资源",
            exact: true,
          }),
        ).toBeVisible();
        await expect(
          page.locator(`[data-panel-id="${target.panel}"]`),
        ).toBeVisible();
        await page.screenshot({
          path: info.outputPath(`${target.type.toLowerCase()}-default.png`),
          fullPage: true,
        });
      }
      expect(errors).toEqual([]);
    });
  }
