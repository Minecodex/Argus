import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const app = process.env.ARGUS_PLAN8_NATIVE_APP;
const api = process.env.ARGUS_PLAN8_NATIVE_API;
const session = process.env.ARGUS_PLAN8_NATIVE_SESSION;
const csrf = process.env.ARGUS_PLAN8_NATIVE_CSRF;
const trace = process.env.ARGUS_PLAN8_NATIVE_TRACE;
test.skip(
  !app || !api || !session || !csrf || !trace,
  "requires a live, owned SDK pipeline and application-reader session",
);

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Plan8 native SDK application ${english ? "en" : "zh"} ${theme}`, async ({
      page,
      context,
    }, info) => {
      const text = (zh: string, en: string) => (english ? en : zh);
      const errors: string[] = [],
        apiOrigins = new Set<string>();
      page.on("pageerror", (error) => errors.push(error.message));
      page.on("request", (request) => {
        if (request.url().includes("/api/v1/"))
          apiOrigins.add(new URL(request.url()).origin);
      });
      await context.addCookies([
        {
          name: "argus_enterprise_session",
          value: session!,
          domain: "127.0.0.1",
          path: "/",
          httpOnly: true,
          sameSite: "Lax",
        },
        {
          name: "argus_enterprise_csrf",
          value: csrf!,
          domain: "127.0.0.1",
          path: "/",
          sameSite: "Lax",
        },
      ]);
      await page.setViewportSize({ width: 1440, height: 1100 });
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.goto(`/applications/${app}`);
      const region = page.getByRole("region", {
        name: text("应用性能", "Application performance"),
        exact: true,
      });
      await expect(region).toBeVisible();
      await expect(region.getByRole("table")).toContainText("payments");
      await expect(region.getByRole("table")).toContainText("inventory");
      for (const [label, field] of [
        [text("运行实例", "Runtime instances"), "queryAPMInstances"],
        [text("接口", "Endpoints"), "queryAPMEndpoints"],
        [text("请求与耗时", "Requests and duration"), "queryAPMRED"],
        [text("服务拓扑", "Service topology"), "queryAPMTopology"],
      ]) {
        const loaded = page.waitForResponse(
          (response) =>
            response.url().endsWith("/enterprise/traces/graphql") &&
            response.request().method() === "POST" &&
            String(response.request().postDataJSON()?.query).includes(field!),
        );
        await region
          .getByRole("radiogroup")
          .getByText(label, { exact: true })
          .click();
        const response = await loaded;
        expect(response.status()).toBe(200);
        const actual = (await response.json()).data[field!];
        expect(actual.status).toBe("available");
        expect(
          field === "queryAPMTopology"
            ? actual.nodes.length
            : actual.rows.length,
        ).toBeGreaterThan(0);
        await expect(
          region.getByRole("radio", { name: label, exact: true }),
        ).toBeChecked();
        await expect(region.getByRole("alert")).toHaveCount(0);
      }
      await region
        .getByRole("radiogroup")
        .getByText(text("调用链", "Traces"), { exact: true })
        .click();
      const traceRow = region
        .getByRole("row")
        .filter({ has: page.getByText("GET /checkout", { exact: true }) });
      await expect(traceRow).toBeVisible();
      await traceRow
        .getByRole("button", {
          name: text("打开详情", "Open details"),
          exact: true,
        })
        .click();
      const detail = page.getByRole("dialog", {
        name: text("调用链详情", "Trace details"),
        exact: true,
      });
      await expect(detail).toContainText(trace!);
      await expect(detail).toContainText("GET /checkout");
      await expect(detail).toContainText("GET /inventory");
      await expect(detail).not.toContainText("partner-private-handler");
      await page.keyboard.press("Escape");
      await region
        .getByRole("radiogroup")
        .getByText(text("应用日志", "Application logs"), { exact: true })
        .click();
      await expect(region).toContainText("sdk-checkout-event");
      await expect(region).toContainText("sdk-inventory-event");
      await expect(region).not.toContainText("sdk-partner-event");
      await region
        .getByRole("button", {
          name: text("选择服务（当前全部）", "Choose service (currently all)"),
          exact: true,
        })
        .click();
      const picker = page.getByRole("dialog", {
        name: text("选择服务", "Choose a service"),
        exact: true,
      });
      await picker
        .getByRole("textbox", { name: text("搜索列表", "Search list") })
        .fill("api");
      await picker
        .getByRole("row")
        .filter({ has: page.getByText("inventory", { exact: true }) })
        .getByRole("button", { name: /^(Select|选择) / })
        .click();
      await expect(region).toContainText("sdk-inventory-event");
      await expect(region).not.toContainText("sdk-checkout-event");
      await region
        .getByRole("button", {
          name: text("清除服务筛选", "Clear service filter"),
          exact: true,
        })
        .click();
      await expect(region).toContainText("sdk-checkout-event");
      for (const width of [1280, 1440, 1920]) {
        await page.setViewportSize({ width, height: 1100 });
        expect(
          await page.evaluate(
            () =>
              document.documentElement.scrollWidth <=
              document.documentElement.clientWidth,
          ),
        ).toBe(true);
        const accessibility = await new AxeBuilder({ page }).analyze();
        expect(
          accessibility.violations.filter(
            (item) => item.impact === "critical" || item.impact === "serious",
          ),
        ).toEqual([]);
        await page.screenshot({
          path: info.outputPath(`application-native-sdk-${width}.png`),
          fullPage: true,
        });
      }
      expect(apiOrigins.size).toBeGreaterThan(0);
      expect([...apiOrigins]).toEqual([new URL(api!).origin]);
      expect(errors).toEqual([]);
    });
  }
