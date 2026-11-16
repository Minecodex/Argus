import { expect, test } from "@playwright/test";

for (const english of [false, true]) {
  for (const theme of ["light", "dark"]) {
    test(`conversation file formats ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.goto("/login");
      await page.getByLabel(english ? "Username" : "用户名").fill("root");
      await page.getByLabel(english ? "Password" : "密码").fill("123456");
      await page.locator('form button[type="submit"]').click();
      await expect(page.locator(".argus-chat-composer")).toBeVisible();
      const initialComposer = page.locator(".argus-chat-composer");
      await initialComposer.getByRole("textbox").fill("检查主机状态");
      await initialComposer
        .getByRole("button", { name: english ? "Send" : "发送", exact: true })
        .click();
      await expect(page).toHaveURL(/\?c=/);
      await expect(initialComposer.getByRole("textbox")).toHaveValue("");
      await expect(
        page.getByRole("button", {
          name: english ? "Stop" : "停止",
          exact: true,
        }),
      ).toHaveCount(0);
      const files = [
        {
          name: "measurements.csv",
          mimeType: "text/csv",
          buffer: Buffer.from(
            'name,note,value\r\n"Redis, native","first\nsecond",37\r\n',
          ),
        },
        {
          name: "report.md",
          mimeType: "text/markdown",
          buffer: Buffer.from(
            '# Preview report\n\n**Measured**\n\n|Metric|Value|\n|---|---|\n|commands|37|\n\n```json\n{"peak_memory_bytes":1562696}\n```',
          ),
        },
        {
          name: "records.json",
          mimeType: "application/json",
          buffer: Buffer.from('{"large":900719925474099312345}'),
        },
        {
          name: "unsafe.html",
          mimeType: "text/html",
          buffer: Buffer.from(
            "<h1>Not rendered HTML</h1><script>window.previewUnsafe=true</script>",
          ),
        },
        {
          name: "document.pdf",
          mimeType: "application/pdf",
          buffer: Buffer.from("%PDF-1.4\n"),
        },
        {
          name: "measurements.tsv",
          mimeType: "text/tab-separated-values",
          buffer: Buffer.from("name\tvalue\nRedis\t37\n"),
        },
        {
          name: "events.ndjson",
          mimeType: "application/x-ndjson",
          buffer: Buffer.from('{"count":900719925474099312345}\n{"count":2}'),
        },
        {
          name: "config.yaml",
          mimeType: "application/yaml",
          buffer: Buffer.from("service:\n  name: redis\n"),
        },
        {
          name: "pixel.png",
          mimeType: "image/png",
          buffer: Buffer.from(
            "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a0boAAAAASUVORK5CYII=",
            "base64",
          ),
        },
      ];
      await page.locator('input[type="file"]').setInputFiles(files);
      await expect(page.locator(".argus-chat-chip")).toHaveCount(files.length);
      const composer = page.locator(".argus-chat-composer");
      await composer.getByRole("textbox").fill("检查主机状态");
      await composer
        .getByRole("button", { name: english ? "Send" : "发送", exact: true })
        .click();
      await expect(composer.getByRole("textbox")).toHaveValue("", {
        timeout: 20000,
      });
      await page.reload();
      await page
        .getByRole("button", {
          name: english ? "Pinned summary" : "置顶摘要",
          exact: true,
        })
        .click();
      await page.locator('summary[title="/workspace/uploads"]').click();
      for (const folder of await page
        .locator('summary[title^="/workspace/uploads/"]')
        .all())
        await folder.click();
      await page
        .getByRole("button", {
          name: `${english ? "Preview" : "预览"} measurements.csv`,
          exact: true,
        })
        .click();
      let dialog = page.getByRole("dialog", {
        name: "measurements.csv",
        exact: true,
      });
      await expect(dialog.getByRole("table")).toBeVisible();
      await expect(
        dialog.getByRole("cell", { name: "Redis, native", exact: true }),
      ).toBeVisible();
      await expect(
        dialog.getByRole("cell", { name: "first second", exact: true }),
      ).toBeVisible();
      await dialog
        .getByRole("button", { name: english ? "Raw" : "原文", exact: true })
        .click();
      await expect(dialog.locator("pre")).toContainText('"Redis, native"');
      await dialog
        .getByRole("button", {
          name: english ? "Preview" : "预览",
          exact: true,
        })
        .click();
      await page.screenshot({
        path: info.outputPath("csv-table.png"),
        animations: "disabled",
      });
      await page.keyboard.press("Escape");
      await page
        .getByRole("button", {
          name: `${english ? "Preview" : "预览"} report.md`,
          exact: true,
        })
        .last()
        .click();
      dialog = page.getByRole("dialog", { name: "report.md", exact: true });
      await expect(
        dialog.getByRole("heading", { name: "Preview report", exact: true }),
      ).toBeVisible();
      await expect(dialog.getByRole("table")).toContainText("commands");
      await expect(dialog.locator("pre")).toContainText("1562696");
      await page.screenshot({
        path: info.outputPath("markdown-report.png"),
        animations: "disabled",
      });
      await page.keyboard.press("Escape");
      for (const [name, content] of [
        ["records.json", "900719925474099312345"],
        ["unsafe.html", "<script>"],
        ["document.pdf", english ? "download" : "下载"],
        ["events.ndjson", "900719925474099312345"],
        ["config.yaml", "name: redis"],
      ]) {
        await page
          .getByRole("button", {
            name: `${english ? "Preview" : "预览"} ${name}`,
            exact: true,
          })
          .last()
          .click();
        dialog = page.getByRole("dialog", { name, exact: true });
        await expect(dialog).toContainText(content);
        expect(await page.evaluate(() => "previewUnsafe" in window)).toBe(
          false,
        );
        await page.keyboard.press("Escape");
      }
      await page
        .getByRole("button", {
          name: `${english ? "Preview" : "预览"} measurements.tsv`,
          exact: true,
        })
        .click();
      dialog = page.getByRole("dialog", {
        name: "measurements.tsv",
        exact: true,
      });
      await expect(dialog.getByRole("table")).toContainText("Redis");
      await page.keyboard.press("Escape");
      await page
        .getByRole("button", {
          name: `${english ? "Preview" : "预览"} pixel.png`,
          exact: true,
        })
        .click();
      dialog = page.getByRole("dialog", { name: "pixel.png", exact: true });
      await expect(
        dialog.getByRole("img", { name: "pixel.png", exact: true }),
      ).toBeVisible();
      expect(
        await dialog
          .locator("img")
          .evaluate(
            (img: HTMLImageElement) => img.complete && img.naturalWidth === 1,
          ),
      ).toBe(true);
      await page.keyboard.press("Escape");
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
    });
  }
}
