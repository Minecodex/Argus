import { expect, test } from "@playwright/test";
import { openSummarySection } from "./helpers/conversation-summary";

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`summary resource table ${width} ${english ? "en" : "zh"} ${theme}`, async ({
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
        const composer = page.locator(".argus-chat-composer");
        await composer.getByRole("textbox").fill("Summary resources");
        await composer
          .getByRole("button", { name: english ? "Send" : "发送", exact: true })
          .click();
        await expect(page).toHaveURL(/c=/);
        await expect(composer.getByRole("textbox")).toHaveValue("");
        const conversationId = new URL(page.url()).searchParams.get("c")!;
        await page.evaluate(
          ({ conversationId }) => {
            const key = "argus-mock:db-v14",
              db = JSON.parse(localStorage.getItem(key)!),
              hint = JSON.parse(
                localStorage.getItem("argus.auth_hint.enterprise")!,
              ),
              at = new Date().toISOString();
            const enterprise = db.enterpriseUsers.find(
                (row: { userId: string }) => row.userId === hint.user_id,
              ).enterpriseId,
              seed = db.hosts.find(
                (row: { enterpriseId: string }) =>
                  row.enterpriseId === enterprise,
              );
            const refs: Array<{ id: string; type: string }> = [];
            for (let i = 1; i <= 8; i++) {
              const id = `summary-host-${i}`;
              db.hosts.push({
                ...seed,
                id,
                name: `Summary Host ${String(i).padStart(2, "0")}`,
              });
              refs.push({ id, type: "host" });
            }
            db.monitoring ??= {};
            const domain = (db.monitoring[enterprise] ??= {
              applications: [],
              services: [],
              bindings: [],
              targets: [],
            });
            for (let i = 1; i <= 4; i++) {
              const id = `summary-app-${i}`;
              domain.applications.push({
                id,
                name: `Summary App ${String(i).padStart(2, "0")}`,
                application_key: id,
                environment: "production",
                status: "active",
                version: 1,
                created_at: at,
                updated_at: at,
              });
              refs.push({ id, type: "application" });
            }
            db.dataAuthorizationGrants ??= [];
            for (const ref of refs)
              db.dataAuthorizationGrants.push({
                subject_type: "user",
                subject_id: hint.user_id,
                resource_type: ref.type,
                resource_id: ref.id,
                active: true,
              });
            db.messages = db.messages.filter(
              (row: { conversationId: string }) =>
                row.conversationId !== conversationId,
            );
            db.messages.push({
              id: "references",
              conversationId,
              role: "user",
              content: "Referenced resources",
              createdAt: at,
              dashboardContext: {
                mode: "none",
                dashboard_ids: [],
                resource_refs: refs,
              },
            });
            localStorage.setItem(key, JSON.stringify(db));
          },
          { conversationId },
        );
        await page.reload();
        const trigger = page.getByRole("button", {
          name: english ? "Pinned summary" : "置顶摘要",
          exact: true,
        });
        await trigger.click();
        const popup = page.getByRole("dialog", {
          name: english ? "Pinned summary" : "置顶摘要",
          exact: true,
        });
        const sections = popup.locator(
          ".argus-conversation-workspace > details",
        );
        await expect(sections).toHaveCount(3);
        await expect(
          popup.locator(".argus-conversation-workspace > details[open]"),
        ).toHaveCount(0);
        await openSummarySection(page, "resources");
        const resource = page.getByTestId("conversation-resources-section"),
          rows = resource.locator("tbody tr");
        await expect(rows).toHaveCount(5);
        await expect(
          resource.getByRole("link", { name: "Summary Host 01", exact: true }),
        ).toHaveAttribute("href", "/hosts/summary-host-1");
        await resource
          .getByRole("button", {
            name: english ? "Next page" : "下一页",
            exact: true,
          })
          .click();
        await expect(rows).toHaveCount(5);
        await expect(
          resource.getByRole("link", { name: "Summary Host 06", exact: true }),
        ).toBeVisible();
        const search = resource.getByRole("searchbox", {
          name: english ? "Search resource names" : "按资源名称搜索",
        });
        await search.fill("Host 08");
        await expect(rows).toHaveCount(1);
        await expect(
          resource.getByRole("link", { name: "Summary Host 08", exact: true }),
        ).toBeVisible();
        await search.fill("");
        await resource
          .getByRole("button", {
            name: english ? /Category$/ : /分类$/,
          })
          .click();
        await page
          .getByRole("option", {
            name: english ? "Application" : "应用",
            exact: true,
          })
          .click();
        await expect(rows).toHaveCount(4);
        await expect(
          resource.getByRole("link", { name: "Summary App 01", exact: true }),
        ).toHaveAttribute("href", "/applications/summary-app-1");
        await search.fill("missing-resource");
        await expect(rows).toHaveCount(0);
        await expect(resource).toContainText(
          english ? "No matching resource references" : "没有匹配的引用资源",
        );
        await search.fill("");
        expect(
          await popup.evaluate((el) => el.scrollWidth <= el.clientWidth),
        ).toBe(true);
        await page.screenshot({
          path: info.outputPath("summary-table.png"),
          animations: "disabled",
        });
        await page.keyboard.press("Escape");
        await trigger.click();
        await expect(
          popup.locator(".argus-conversation-workspace > details[open]"),
        ).toHaveCount(0);
      });
    }
