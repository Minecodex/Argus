import { test, expect } from "@playwright/test";
import fs from "node:fs";
const ready = Boolean(
  process.env.ARGUS_E2E_EXTERNAL === "1" &&
  process.env.ARGUS_PLAN8_PICKER_METADATA &&
  process.env.ARGUS_PLAN8_NATIVE_SESSION,
);
const meta = ready
  ? JSON.parse(
      fs.readFileSync(process.env.ARGUS_PLAN8_PICKER_METADATA!, "utf8"),
    )
  : {};
test.skip(
  !ready,
  "requires the owned live metadata and native SDK API fixture",
);
for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`actual paged metadata picker ${english ? "en" : "zh"} ${theme}`, async ({
      page,
      context,
    }, info) => {
      const text = (zh: string, en: string) => (english ? en : zh);
      const api = process.env.ARGUS_PLAN8_NATIVE_API!;
      await context.addCookies([
        {
          name: "argus_enterprise_session",
          value: process.env.ARGUS_PLAN8_NATIVE_SESSION!,
          domain: "127.0.0.1",
          path: "/",
          httpOnly: true,
          sameSite: "Lax",
        },
        {
          name: "argus_enterprise_csrf",
          value: process.env.ARGUS_PLAN8_NATIVE_CSRF!,
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
      const lists: Array<{ url: string; count: number; ids: string[] }> = [];
      page.on("response", async (r) => {
        const u = new URL(r.url());
        if (
          r.request().method() === "GET" &&
          r.status() === 200 &&
          (u.pathname.endsWith("/applications") ||
            u.pathname.endsWith("/services"))
        ) {
          const d = await r.json();
          if (Array.isArray(d.items))
            lists.push({
              url: r.url(),
              count: d.items.length,
              ids: d.items.map((x: any) => x.id),
            });
        }
      });
      const control = async (operation: string) => {
        const r = await page.request.post(
          process.env.ARGUS_PLAN8_PICKER_CONTROL!,
          {
            data: { operation },
            headers: {
              "X-Owned-Fixture": process.env.ARGUS_PLAN8_PICKER_NONCE!,
            },
          },
        );
        expect(r.status()).toBe(204);
      };
      await page.goto("/monitoring");
      await page
        .getByRole("button", {
          name: text("新建监控", "New monitor"),
          exact: true,
        })
        .click();
      const types = page.getByRole("dialog", {
        name: text("选择监控类型", "Choose a monitoring type"),
      });
      await types
        .getByRole("row")
        .filter({ has: page.getByText("MySQL", { exact: true }) })
        .getByRole("button")
        .click();
      const form = page.getByRole("dialog", {
        name: text("新建 MySQL 监控", "New MySQL monitor"),
        exact: true,
      });
      const draft = form.getByRole("textbox", {
        name: new RegExp(text("监控名称", "Monitor name")),
      });
      await draft.fill("Preserved actual picker draft");
      const openApps = async () => {
        await form
          .getByRole("button", {
            name: text("选择应用", "Choose applications"),
            exact: true,
          })
          .click();
        return page.getByRole("dialog", {
          name: text("选择应用", "Choose applications"),
          exact: true,
        });
      };
      let picker = await openApps();
      const search = () =>
        picker.getByRole("textbox", { name: text("搜索列表", "Search list") });
      const appButton = (name: string) =>
        picker.getByRole("button", {
          name: new RegExp(`^(选择|Select) ${name} `),
        });
      await search().fill("PickerMeta 000");
      await appButton("PickerMeta 000").click();
      await search().fill("PickerMeta");
      await picker
        .getByRole("button", { name: text("下一页", "Next"), exact: true })
        .click();
      const second = picker
        .locator('button[aria-pressed="false"][aria-label*="PickerMeta "]')
        .first();
      await expect(second).toBeVisible();
      const secondName = (await second.getAttribute("aria-label"))!.match(
        /PickerMeta \d{3}/,
      )![0];
      expect(secondName).not.toBe("PickerMeta 000");
      await second.click();
      await picker
        .getByRole("button", { name: text("上一页", "Previous"), exact: true })
        .click();
      await search().fill("PickerMeta 000");
      await expect(appButton("PickerMeta 000")).toHaveAttribute(
        "aria-pressed",
        "true",
      );
      await page.keyboard.press("Escape");
      await expect(picker).toBeHidden();
      await expect(draft).toHaveValue("Preserved actual picker draft");
      await expect(
        form.getByText("PickerMeta 000", { exact: true }),
      ).toHaveCount(0);
      picker = await openApps();
      await search().fill("PickerMeta 000");
      await expect(appButton("PickerMeta 000")).toHaveAttribute(
        "aria-pressed",
        "false",
      );
      await appButton("PickerMeta 000").click();
      await search().fill(secondName);
      await appButton(secondName).click();
      await picker
        .getByRole("button", { name: /^(确认选择|Confirm selection)/ })
        .click();
      await expect(
        form.getByText("PickerMeta 000", { exact: true }),
      ).toBeVisible();
      await expect(form.getByText(secondName, { exact: true })).toBeVisible();
      await expect(
        form.getByRole("textbox", {
          name: text("环境", "Environment"),
          exact: true,
        }),
      ).toHaveCount(0);
      const association = form
        .locator(".argus-monitoring-association")
        .filter({ hasText: "PickerMeta 000" });
      await association
        .getByRole("button", {
          name: text("选择服务（可选）", "Choose a service (optional)"),
          exact: true,
        })
        .click();
      let services = page.getByRole("dialog", {
        name: text("选择服务", "Choose a service"),
        exact: true,
      });
      await services
        .getByRole("button", { name: text("下一页", "Next"), exact: true })
        .click();
      const serviceButton = services
        .getByRole("button", { name: /^(选择|Select) PickerSvc / })
        .first();
      await expect(serviceButton).toBeVisible();
      const serviceName = (await serviceButton.getAttribute(
        "aria-label",
      ))!.replace(/^(选择|Select) /, "");
      await serviceButton.click();
      await expect(
        association.getByRole("button", { name: serviceName, exact: true }),
      ).toBeVisible();
      await association
        .getByRole("button", { name: serviceName, exact: true })
        .click();
      services = page.getByRole("dialog", {
        name: text("选择服务", "Choose a service"),
        exact: true,
      });
      await services
        .getByRole("textbox", { name: text("搜索列表", "Search list") })
        .fill("PickerSvc 079");
      await expect(
        services.getByRole("button", { name: /^(选择|Select) PickerSvc 079$/ }),
      ).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(
        association.getByRole("button", { name: serviceName, exact: true }),
      ).toBeVisible();
      // Delay the actual response; do not replace its data or status.
      let release!: () => void;
      const delayed = new Promise<void>((resolve) => {
        release = resolve;
      });
      let finishRoute!: () => void;
      const routeSettled = new Promise<void>((resolve) => {
        finishRoute = resolve;
      });
      let oldResponseOutcome = "pending";
      let held = false;
      const pattern = "**/applications/services?**";
      await page.route(
        pattern,
        async (route) => {
          expect(
            new URL(route.request().url()).searchParams.get("application_ids"),
          ).toBe(meta.service_application);
          const response = await route.fetch();
          held = true;
          await delayed;
          try {
            await route.fulfill({ response });
            oldResponseOutcome = "released";
          } catch (error) {
            if (
              !/cancelled|canceled|Target closed|Target page, context|already handled/i.test(
                String(error),
              )
            )
              throw error;
            oldResponseOutcome = "aborted_by_context";
          } finally {
            finishRoute();
          }
        },
        { times: 1 },
      );
      await association
        .getByRole("button", { name: serviceName, exact: true })
        .click();
      await page
        .getByRole("dialog", {
          name: text("选择服务", "Choose a service"),
          exact: true,
        })
        .getByRole("textbox", { name: text("搜索列表", "Search list") })
        .fill("PickerSvc 0");
      await expect
        .poll(() => held, {
          timeout: 5000,
          message: "actual delayed service request must be observed",
        })
        .toBe(true);
      await page.keyboard.press("Escape");
      const otherAssociation = form
        .locator(".argus-monitoring-association")
        .filter({ hasText: secondName });
      await otherAssociation
        .getByRole("button", {
          name: text("选择服务（可选）", "Choose a service (optional)"),
          exact: true,
        })
        .click();
      services = page.getByRole("dialog", {
        name: text("选择服务", "Choose a service"),
        exact: true,
      });
      await expect(services.getByRole("status")).toContainText(
        text("没有匹配的结果", "No matching results"),
      );
      release();
      await routeSettled;
      expect(["released", "aborted_by_context"]).toContain(oldResponseOutcome);
      await expect(
        services.getByRole("button", { name: /PickerSvc/ }),
      ).toHaveCount(0);
      await page.keyboard.press("Escape");
      await expect(
        association.getByRole("button", { name: serviceName, exact: true }),
      ).toBeVisible();
      await form
        .getByRole("textbox", {
          name: new RegExp(text("服务地址", "Service endpoint")),
        })
        .fill("owned-unused-picker.test:3306");
      await form
        .getByRole("textbox", { name: new RegExp(text("用户名", "Username")) })
        .fill("monitor");
      await form
        .getByRole("button", {
          name: text("选择执行收集器", "Choose a Collector"),
          exact: true,
        })
        .click();
      const collectors = page.getByRole("dialog", {
        name: text("选择执行收集器", "Choose a Collector"),
        exact: true,
      });
      await collectors
        .getByRole("button", { name: new RegExp(meta.collector_name) })
        .click();
      const request = page.waitForRequest(
        (r) =>
          r.method() === "POST" &&
          new URL(r.url()).pathname.endsWith("/monitoring/commands/preview"),
      );
      await form
        .getByRole("button", {
          name: text("预览采集配置", "Preview configuration"),
          exact: true,
        })
        .click();
      const actualRequest = await request;
      const submitted = actualRequest.postDataJSON();
      const previewResponse = await actualRequest.response();
      expect(previewResponse?.status()).toBe(503);
      expect(submitted.collection.collector_id).toBe(meta.collector_id);
      const wanted = meta.applications
        .filter((a: any) => ["PickerMeta 000", secondName].includes(a.name))
        .map((a: any) => a.id)
        .sort();
      expect(
        submitted.collection.bindings.map((b: any) => b.application_id).sort(),
      ).toEqual(wanted);
      const service = meta.services.find((s: any) => s.name === serviceName);
      expect(
        submitted.collection.bindings.find(
          (b: any) => b.application_id === meta.service_application,
        ).service_id,
      ).toBe(service.id);
      for (const binding of submitted.collection.bindings)
        expect(
          Object.keys(binding).every((k) =>
            ["application_id", "service_id"].includes(k),
          ),
        ).toBe(true);
      expect(submitted.collection).not.toHaveProperty("environment");
      // This fixture exercises picker serialization, and never confirms a monitor.
      try {
        await control("revoke");
        const denied = await page.request.get(
          api + "/api/v1/applications/" + meta.revoked_candidate,
        );
        expect(denied.status()).toBe(403);
        picker = await openApps();
        await picker
          .getByRole("textbox", { name: text("搜索列表", "Search list") })
          .fill("PickerMeta 031");
        await expect(
          picker.getByRole("button", {
            name: /^(选择|Select) PickerMeta 031 /,
          }),
        ).toHaveCount(0);
        await expect(picker.getByRole("status")).toContainText(
          text("没有匹配的结果", "No matching results"),
        );
        await page.keyboard.press("Escape");
        await expect(draft).toHaveValue("Preserved actual picker draft");
      } finally {
        await control("restore");
      }
      try {
        await control("archive");
        const actual = await page.request.get(
          api + "/api/v1/applications?search=PickerMeta+031&limit=30",
        );
        expect(actual.status()).toBe(200);
        expect((await actual.json()).items).toEqual([]);
        picker = await openApps();
        await picker
          .getByRole("textbox", { name: text("搜索列表", "Search list") })
          .fill("031");
        // Search also matches application keys; a valid other application
        // must remain visible when its key contains this numeric fragment.
        await expect(appButton("PickerMeta 010")).toBeVisible();
        await picker
          .getByRole("textbox", { name: text("搜索列表", "Search list") })
          .fill("PickerMeta 031");
        await expect(
          picker.getByRole("button", {
            name: /^(选择|Select) PickerMeta 031 /,
          }),
        ).toHaveCount(0);
        await expect(picker.getByRole("status")).toContainText(
          text("没有匹配的结果", "No matching results"),
        );
        await page.keyboard.press("Escape");
        await expect(draft).toHaveValue("Preserved actual picker draft");
      } finally {
        await control("activate");
      }
      const foreign = await page.request.get(
        api + "/api/v1/applications/" + meta.foreign_application,
      );
      expect(foreign.status()).toBe(403);
      expect(lists.length).toBeGreaterThan(3);
      expect(lists.every((x) => x.count <= 30)).toBe(true);
      expect(lists.some((x) => new URL(x.url).searchParams.has("cursor"))).toBe(
        true,
      );
      expect(
        lists.every((x) => new URL(x.url).origin === new URL(api).origin),
      ).toBe(true);
      await fs.promises.writeFile(
        info.outputPath("actual-picker-evidence.json"),
        JSON.stringify({
          passed: true,
          scope: meta.scope,
          locale: english ? "en" : "zh",
          theme,
          actual_http_lists: lists,
          controlled_grant_revocation: true,
          controlled_application_archive: true,
          metadata_applications: 80,
          metadata_services: 80,
          no_monitor_confirmed: true,
          stable_ids_serialized: true,
          stale_service_response_fenced: true,
          old_response_outcome: oldResponseOutcome,
        }),
      );
      await page.screenshot({
        path: info.outputPath("actual-picker.png"),
        fullPage: true,
      });
    });
  }
