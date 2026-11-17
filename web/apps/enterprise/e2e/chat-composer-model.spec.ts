import { expect, test } from "@playwright/test";

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`composer model picker ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 1000 });
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
        await page
          .getByRole("button", {
            name: english ? "Sign in" : "登录",
            exact: true,
          })
          .click();
        await expect(page).not.toHaveURL(/\/login/);
        await expect(page.locator(".argus-chat-composer")).toBeVisible();
        await page.evaluate(() => {
          const key = "argus-mock:db-v14",
            db = JSON.parse(localStorage.getItem(key)!),
            hint = JSON.parse(
              localStorage.getItem("argus.auth_hint.enterprise")!,
            );
          const seed = db.models.find(
            (model: {
              enterpriseId: string;
              enabled: boolean;
              healthStatus: string;
            }) =>
              model.enterpriseId ===
                db.enterpriseUsers.find(
                  (row: { userId: string }) => row.userId === hint.user_id,
                ).enterpriseId &&
              model.enabled &&
              model.healthStatus === "healthy",
          );
          for (const [id, name, enabled] of [
            ["composer-model-alpha", "Test model Alpha", true],
            [
              "composer-model-beta",
              "Test model Beta with an intentionally long display name",
              true,
            ],
            ["composer-model-disabled", "Unavailable test model", false],
          ])
            db.models.push({ ...seed, id, name, enabled });
          localStorage.setItem(key, JSON.stringify(db));
          localStorage.setItem("argus.lastModelId", "composer-model-alpha");
        });
        await page.reload();
        const composer = page.locator(".argus-chat-composer"),
          input = composer.getByRole("textbox"),
          picker = composer
            .locator(".argus-chat-composer__model")
            .getByRole("button");
        await expect(picker).toContainText("Test model Alpha");
        const measure = () =>
          picker.evaluate((el) => {
            const value = el
                .querySelector(".argus-select__value")!
                .getBoundingClientRect(),
              arrow = el
                .querySelector(".argus-select__indicator")!
                .getBoundingClientRect();
            return {
              width: el.getBoundingClientRect().width,
              gap: arrow.left - value.right,
              overflow: el.scrollWidth > el.clientWidth,
            };
          });
        const compact = await measure();
        expect(compact.width).toBeLessThan(240);
        expect(compact.gap).toBeGreaterThanOrEqual(0);
        expect(compact.gap).toBeLessThanOrEqual(12);

        await expect(picker).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
        await picker.hover();
        await expect(picker).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
        await input.fill("Keep this unsent draft");
        await picker.click();
        const listbox = page.getByRole("listbox");
        await expect(listbox).toBeVisible();
        await expect(
          page.getByRole("option", { name: /Unavailable test model/ }),
        ).toHaveAttribute("aria-disabled", "true");
        await expect
          .poll(async () => {
            const menu = (await listbox.boundingBox())!,
              trigger = (await picker.boundingBox())!;
            return menu.y + menu.height - trigger.y;
          })
          .toBeLessThanOrEqual(1);
        await page
          .getByRole("option", {
            name: "Test model Beta with an intentionally long display name",
            exact: true,
          })
          .click();
        await expect(picker).toContainText(
          "Test model Beta with an intentionally long display name",
        );
        const long = await measure();
        expect(long.width).toBeLessThanOrEqual(261);
        expect(long.gap).toBeLessThanOrEqual(12);
        expect(long.overflow).toBe(false);
        await expect(input).toHaveValue("Keep this unsent draft");
        await expect(page).not.toHaveURL(/c=/);
        await composer
          .getByRole("button", { name: english ? "Send" : "发送", exact: true })
          .click();
        await expect(page).toHaveURL(/c=/);
        await expect(input).toHaveValue("");
        await expect(picker).toContainText(
          "Test model Beta with an intentionally long display name",
        );
        await expect(
          page.locator(".argus-chat__header .argus-select"),
        ).toHaveCount(0);
        await expect(
          page.getByRole("button", {
            name: english ? /Select model$/ : /选择模型$/,
          }),
        ).toHaveCount(1);
        await picker.click();
        await page
          .getByRole("option", { name: "Test model Alpha", exact: true })
          .click();
        await expect(picker).toContainText("Test model Alpha");
        const stored = await page.evaluate(() => {
          const db = JSON.parse(localStorage.getItem("argus-mock:db-v14")!);
          return db.conversations.find(
            (c: { id: string }) =>
              c.id === new URL(location.href).searchParams.get("c"),
          ).selectedModelId;
        });
        expect(stored).toBe("composer-model-alpha");
        await page.reload();
        await expect(picker).toContainText("Test model Alpha");
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        await page.screenshot({
          path: info.outputPath("composer-model.png"),
          animations: "disabled",
        });
      });
    }
