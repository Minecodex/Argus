import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 Kubernetes terminal and port forward ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const errors: string[] = [],
        inputs: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.setViewportSize({ width: 1440, height: 1100 });
      await page.routeWebSocket("wss://argus-mock.invalid/**", (socket) => {
        let sequence = 0;
        const send = (value: object) =>
          socket.send(
            JSON.stringify({
              ...value,
              protocol: "argus.kubernetes_stream/v1",
              sequence: ++sequence,
            }),
          );
        socket.onMessage((data) => {
          const frame = JSON.parse(String(data)) as Record<string, unknown>;
          if (frame.type === "client_hello") {
            expect(frame.ticket).toHaveLength(43);
            send({
              type: "server_ready",
              mode: "exec",
              nonce: "mock-native-stream-nonce-12345678",
              status: "active",
            });
            send({
              type: "output",
              data: Buffer.from("native terminal fixture\r\n").toString(
                "base64",
              ),
            });
          }
          if (frame.type === "input") {
            inputs.push(String(frame.data));
            send({
              type: "output",
              data: Buffer.from(String(frame.data)).toString("base64"),
            });
          }
          if (frame.type === "close") socket.close();
        });
      });
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
      await page
        .getByRole("button", {
          name: english ? "All authorized namespaces" : "全部已授权命名空间",
          exact: true,
        })
        .click();
      await page
        .getByRole("dialog", {
          name: english ? "Choose a namespace" : "选择命名空间",
          exact: true,
        })
        .getByRole("button", { name: /^(Select|选择) production$/ })
        .click();
      await page
        .getByRole("button", {
          name: new RegExp(english ? "Resource type" : "资源类型"),
        })
        .click();
      await page.getByRole("option", { name: "Pod", exact: true }).click();
      const row = page
        .getByRole("row")
        .filter({ has: page.getByText("pod-example", { exact: true }) });
      await row
        .getByRole("button", {
          name: english ? "Open terminal" : "打开终端",
          exact: true,
        })
        .click();
      let dialog = page.getByRole("dialog", {
        name: english ? "Pod terminal · pod-example" : "Pod 终端 · pod-example",
        exact: true,
      });
      await dialog
        .getByRole("textbox", {
          name: english ? "Reason" : "操作原因",
          exact: true,
        })
        .fill("Investigate an isolated session");
      await dialog
        .getByRole("button", {
          name: english ? "Create session" : "创建会话",
          exact: true,
        })
        .click();
      await expect(
        dialog.getByText(english ? "Connected" : "已连接", { exact: false }),
      ).toBeVisible();
      await expect(dialog.locator(".xterm-helper-textarea")).toHaveCount(1);
      await dialog.locator(".xterm-helper-textarea").focus();
      await page.keyboard.type("pwd");
      await page.keyboard.press("Enter");
      await expect.poll(() => inputs.join("")).toContain("pwd");
      await page.screenshot({
        path: info.outputPath("pod-terminal.png"),
        fullPage: true,
      });
      await dialog
        .getByRole("button", {
          name: english ? "Close session" : "关闭会话",
          exact: true,
        })
        .click();
      await expect(dialog).not.toBeVisible();
      await row
        .getByRole("button", {
          name: english ? "Port forward" : "端口转发",
          exact: true,
        })
        .click();
      dialog = page.getByRole("dialog", {
        name: english ? "Port forward · pod-example" : "端口转发 · pod-example",
        exact: true,
      });
      await dialog
        .getByRole("spinbutton", {
          name: english ? "Target TCP port" : "目标 TCP 端口",
          exact: true,
        })
        .fill("3306");
      await dialog
        .getByRole("textbox", {
          name: english ? "Reason" : "操作原因",
          exact: true,
        })
        .fill("Inspect isolated MySQL connection");
      await dialog
        .getByRole("button", {
          name: english ? "Create session" : "创建会话",
          exact: true,
        })
        .click();
      await expect(dialog.locator("pre")).toContainText(
        "argusctl kubernetes port-forward",
      );
      await expect(dialog.locator("pre")).toContainText(
        "--ticket-stdin --listen 127.0.0.1:18080",
      );
      const token = dialog.getByRole("textbox", {
        name: english ? "One-use connection ticket" : "一次性连接票据",
        exact: true,
      });
      await expect(token).toHaveValue(/^[\w-]{43}$/);
      const old = await token.inputValue();
      await dialog
        .getByRole("button", {
          name: english ? "Replace ticket" : "重新签发票据",
          exact: true,
        })
        .click();
      await expect(token).not.toHaveValue(old);
      expect(
        await page.evaluate(() =>
          JSON.stringify({ local: localStorage, session: sessionStorage }),
        ),
      ).not.toContain(old);
      const violations = (
        await new AxeBuilder({ page }).analyze()
      ).violations.filter(
        (v) => v.impact === "critical" || v.impact === "serious",
      );
      expect(violations).toEqual([]);
      await page.screenshot({
        path: info.outputPath("port-forward.png"),
        fullPage: true,
      });
      await dialog
        .getByRole("button", {
          name: english ? "Close session" : "关闭会话",
          exact: true,
        })
        .click();
      await expect(dialog).not.toBeVisible();
      expect(errors).toEqual([]);
    });
  }
