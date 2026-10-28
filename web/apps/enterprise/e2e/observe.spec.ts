import { test, expect, type Page, type Locator } from "@playwright/test";
import { selectTrigger } from "./helpers/select";
import { setChoice } from "./helpers/choice";
import AxeBuilder from "@axe-core/playwright";
test.use({ actionTimeout: 15000 });

async function login(page: Page, english = false, theme = "light") {
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
    .getByRole("button", { name: english ? "Sign in" : "登录", exact: true })
    .click();
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/observe");
}
async function select(
  page: Page,
  root: Page | Locator,
  label: string,
  option: string,
) {
  await selectTrigger(root, label).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}
const cards = (page: Page) => page.locator(".argus-observe-query");
async function scope(page: Page, root: Locator, english = false, index = 0) {
  await root
    .getByRole("button", { name: english ? /Resources ·/ : /资源范围 ·/ })
    .click();
  const dialog = page.getByRole("dialog", {
    name: english ? "Select resources" : "选择资源范围",
  });
  await expect(dialog).toBeVisible();
  await dialog
    .getByRole("button", {
      name: english ? "Clear selection" : "清空选择",
      exact: true,
    })
    .click();
  const choices = dialog.getByRole("button", {
    name: english ? /^Select / : /^选择 /,
  });
  await choices.nth(index).click();
  await dialog
    .getByRole("button", { name: english ? /Confirm selection/ : /确认选择/ })
    .click();
  await expect(dialog).not.toBeVisible();
}
async function metric(page: Page, root: Locator, english = false) {
  const input = root.getByRole("combobox", {
    name: english ? /^Metric\*?$/ : /^指标\*?$/,
  });
  await input.fill("system_cpu");
  await input.press("ArrowDown");
  await page
    .getByRole("option", { name: /system_cpu_utilization/ })
    .first()
    .click();
  await expect(input).toHaveValue("system_cpu_utilization");
}
async function run(root: Locator, english = false) {
  await root
    .getByRole("button", {
      name: english ? "Run query" : "执行查询",
      exact: true,
    })
    .click();
  await expect(
    root
      .getByRole("status")
      .filter({ hasText: english ? /Complete/ : /查询完成/ }),
  ).toBeVisible();
}

for (const english of [false, true])
  for (const theme of ["light", "dark"])
    test(`observation workspace ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(120000);
      page.setDefaultTimeout(15000);
      await page.setViewportSize({ width: 1440, height: 1000 });
      await login(page, english, theme);
      const tx = (zh: string, en: string) => (english ? en : zh);
      await expect(
        page.getByRole("heading", {
          name: tx("观测查询", "Observability queries"),
          exact: true,
        }),
      ).toBeVisible();
      await expect(cards(page)).toHaveCount(1);
      const a = cards(page).first();
      const toolbar = page.locator(".argus-observe-global.argus-card");
      await expect(toolbar).toBeVisible();
      const hint = a.locator(".argus-observe-scope-hint");
      await expect(hint).toHaveText(
        tx("请先选择资源", "Select resources first"),
      );
      await expect(a.locator(".argus-card__header")).toContainText(
        await hint.innerText(),
      );
      await expect(
        a.locator(".argus-card__header").getByText("promql", { exact: true }),
      ).toHaveCount(0);
      await expect(
        a.getByText(
          tx(
            "临时查询仅保留在当前页面，刷新或离开后清空。",
            "Temporary queries stay on this page and are cleared on reload or leaving.",
          ),
          { exact: true },
        ),
      ).toHaveCount(0);
      await expect(
        a.getByRole("radiogroup", {
          name: tx("结果展示", "Result view"),
          exact: true,
        }),
      ).toHaveCount(0);
      for (const width of [1280, 1440]) {
        await page.setViewportSize({ width, height: 1000 });
        const controls = await Promise.all([
          toolbar.locator(".argus-checkbox").boundingBox(),
          toolbar
            .getByRole("button", {
              name: tx("时间范围", "Time range"),
              exact: true,
            })
            .boundingBox(),
          toolbar
            .getByRole("button", {
              name: tx("执行全部", "Run all"),
              exact: true,
            })
            .boundingBox(),
        ]);
        const centers = controls.map((box) => box!.y + box!.height / 2);
        expect(Math.max(...centers) - Math.min(...centers)).toBeLessThan(8);
        const fields = await a
          .locator(".argus-metric-query-basics > .argus-field")
          .all();
        expect(fields).toHaveLength(4);
        const fieldBoxes = await Promise.all(
          fields.map((field) => field.boundingBox()),
        );
        expect(
          Math.max(...fieldBoxes.map((b) => b!.y)) -
            Math.min(...fieldBoxes.map((b) => b!.y)),
        ).toBeLessThan(8);
        expect(fieldBoxes[1]!.width).toBeLessThanOrEqual(260);
        expect(fieldBoxes[0]!.x).toBeLessThan(fieldBoxes[1]!.x);
        const modeBox = (await a
          .getByRole("radiogroup", {
            name: tx("查询条件", "Query conditions"),
            exact: true,
          })
          .boundingBox())!;
        const runBox = (await a
          .getByRole("button", {
            name: tx("执行查询", "Run query"),
            exact: true,
          })
          .boundingBox())!;
        expect(
          Math.abs(
            modeBox.y + modeBox.height / 2 - runBox.y - runBox.height / 2,
          ),
        ).toBeLessThan(3);
        expect(runBox.x).toBeGreaterThan(modeBox.x + modeBox.width);
      }
      await page.screenshot({
        path: info.outputPath("empty-workspace.png"),
        fullPage: true,
      });
      await expect(
        a.getByRole("button", {
          name: tx("执行查询", "Run query"),
          exact: true,
        }),
      ).toBeDisabled();
      await expect(a.locator('[role="img"]')).toHaveCount(0);
      await expect(
        page.getByRole("button", { name: /保存查询|Save query/ }),
      ).toHaveCount(0);
      await scope(page, a, english);
      await expect(hint).toHaveCount(0);
      await metric(page, a, english);
      await a
        .getByRole("button", {
          name: tx("添加条件", "Add condition"),
          exact: true,
        })
        .click();
      await expect(page.getByRole("dialog")).toHaveCount(0);
      const condition = a.locator(".argus-metric-label-row");
      const label = condition.getByRole("combobox", {
        name: english ? /^Label\*?$/ : /^标签\*?$/,
      });
      await label.fill("sta");
      await label.press("ArrowDown");
      await page.getByRole("option", { name: "state", exact: true }).click();
      const value = condition.getByRole("combobox", {
        name: tx("值", "Value"),
        exact: true,
      });
      await value.fill("i");
      await value.press("ArrowDown");
      await page.getByRole("option", { name: "idle", exact: true }).click();
      await value.press("Tab");
      await expect(
        a.getByRole("group", {
          name: tx("高级查询选项", "Advanced query options"),
          exact: true,
        }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("inline-builder.png"),
        fullPage: true,
      });
      await run(a, english);
      await expect(a.locator('[role="img"]')).toBeVisible();
      await expect(
        a.getByRole("radiogroup", {
          name: tx("结果展示", "Result view"),
          exact: true,
        }),
      ).toBeVisible();
      const expandedHeight = (await a.boundingBox())!.height;
      await a.locator(".argus-observe-query-heading strong").click();
      await expect(
        a.getByRole("button", {
          name: tx("展开查询", "Expand query"),
          exact: true,
        }),
      ).toHaveAttribute("aria-expanded", "false");
      await expect(a.locator(".argus-card__content")).toBeHidden();
      await expect(a.locator('[role="img"]')).toHaveCount(1);
      expect((await a.boundingBox())!.height).toBeLessThan(
        expandedHeight - 100,
      );
      await a
        .getByRole("button", { name: english ? /Resources ·/ : /资源范围 ·/ })
        .click();
      await page
        .getByRole("dialog")
        .getByText(tx("选择资源范围", "Select resources"), { exact: true })
        .click();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: tx("取消", "Cancel"), exact: true })
        .click();
      await expect(a.locator(".argus-card__content")).toBeHidden();
      await a
        .getByRole("button", {
          name: tx("展开查询", "Expand query"),
          exact: true,
        })
        .press("Space");
      await expect(a.locator('[role="img"]')).toBeVisible();
      await expect(
        a.getByRole("status").filter({ hasText: tx("查询完成", "Complete") }),
      ).toBeVisible();
      await a
        .getByRole("button", {
          name: tx("复制查询", "Duplicate query"),
          exact: true,
        })
        .click();
      await expect(cards(page)).toHaveCount(2);
      const b = cards(page).nth(1);
      await run(b, english);
      await a
        .getByRole("button", { name: english ? /Resources ·/ : /资源范围 ·/ })
        .click();
      const dialog = page.getByRole("dialog");
      await dialog
        .getByRole("button", {
          name: tx("清空选择", "Clear selection"),
          exact: true,
        })
        .click();
      await dialog
        .getByRole("button", { name: tx("取消", "Cancel"), exact: true })
        .click();
      await expect(a.locator('[role="img"]')).toBeVisible();
      await expect(b.locator('[role="img"]')).toBeVisible();
      await setChoice(
        a.getByRole("radio", {
          name: tx("DSL 编辑器", "DSL editor"),
          exact: true,
        }),
      );
      const editor = a.getByRole("textbox", {
        name: tx("查询语句", "Query expression"),
        exact: true,
      });
      await expect(editor).toBeVisible();
      await expect(a.locator(".cm-editor")).toBeVisible();
      await editor.fill("system_");
      await expect(editor).toContainText("system_");
      await editor.press("Control+Space");
      await expect(
        page.getByRole("option", { name: /system_cpu_utilization/ }).first(),
      ).toBeVisible();
      await page
        .getByRole("option", { name: /system_cpu_utilization/ })
        .first()
        .click();
      await expect(editor).toContainText("system_cpu_utilization");
      await editor.fill("system_cpu_utilization + 1");
      await a
        .getByRole("radio", {
          name: tx("查询构建器", "Query builder"),
          exact: true,
        })
        .press("Space");
      await expect(
        a.getByText(
          tx(
            "无法无损转换，已保留原查询",
            "Cannot convert losslessly; your query is preserved",
          ),
          { exact: true },
        ),
      ).toBeVisible();
      await expect(editor).toContainText("system_cpu_utilization + 1");
      await page
        .getByRole("button", { name: tx("新建查询", "New query"), exact: true })
        .first()
        .click();
      const c = cards(page).last();
      await select(page, c, tx("查询类型", "Query type"), tx("日志", "Logs"));
      await scope(page, c, english);
      await run(c, english);
      await expect(c.locator(".argus-observation-log-body")).not.toHaveCount(0);
      await select(page, c, tx("查询类型", "Query type"), tx("链路", "Traces"));
      await run(c, english);
      await expect(
        c.getByRole("columnheader", {
          name: tx("调用链 ID", "Trace ID"),
          exact: true,
        }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("workspace.png"),
        fullPage: true,
      });
      await expect(page.locator("body")).not.toHaveJSProperty("scrollWidth", 0);
      const axe = await new AxeBuilder({ page }).analyze();
      expect(
        axe.violations.filter(
          (v) => v.impact === "critical" || v.impact === "serious",
        ),
      ).toEqual([]);
      await page
        .getByRole("tab", {
          name: tx("指标与字段目录", "Metrics & fields"),
          exact: true,
        })
        .click();
      const catalog = page.locator(".argus-observe-catalog");
      await page.setViewportSize({ width: 1920, height: 1000 });
      const filter = catalog.locator(".argus-filter-bar");
      const typeBox = (await selectTrigger(
        filter,
        tx("资源类型", "Resource type"),
      ).boundingBox())!;
      const inlineBox = (await filter
        .locator(".argus-filter-bar__inline-actions")
        .boundingBox())!;
      const timeBox = (await filter
        .getByRole("button", {
          name: tx("时间范围", "Time range"),
          exact: true,
        })
        .boundingBox())!;
      const refreshBox = (await filter
        .getByRole("button", { name: tx("刷新", "Refresh"), exact: true })
        .boundingBox())!;
      expect(inlineBox.x - typeBox.x - typeBox.width).toBeLessThanOrEqual(12);
      expect(timeBox.x).toBeGreaterThan(inlineBox.x + inlineBox.width);
      expect(refreshBox.x - timeBox.x - timeBox.width).toBeLessThanOrEqual(12);
      expect(Math.abs(timeBox.y - refreshBox.y)).toBeLessThan(3);
      await expect(page.locator(".argus-observe-global")).toHaveCount(0);
      await expect(
        catalog.locator(".argus-filter-bar").getByRole("button", {
          name: tx("时间范围", "Time range"),
          exact: true,
        }),
      ).toBeVisible();
      await scope(page, catalog, english);
      await expect(
        catalog.getByRole("button", {
          name: "system_cpu_utilization",
          exact: true,
        }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("catalog-filters.png"),
        fullPage: true,
      });
      await page.reload();
      await expect(cards(page)).toHaveCount(1);
      await expect(cards(page).first().locator('[role="img"]')).toHaveCount(0);
    });

test("resource pages replace query tabs with a scoped entry", async ({
  page,
}) => {
  await login(page);
  for (const path of ["/hosts/host-app-bj-01", "/kubernetes/k8s-prod-east"]) {
    await page.goto(path);
    for (const name of ["指标", "日志", "链路"])
      await expect(page.getByRole("tab", { name, exact: true })).toHaveCount(0);
    await page.getByRole("button", { name: "观测查询", exact: true }).click();
    await expect(page).toHaveURL(/\/observe\?/);
    await expect(
      cards(page)
        .first()
        .getByRole("button", { name: /资源范围 · 已选 1 项/ }),
    ).toBeVisible();
  }
});

test("panel scopes, explicit empty selection and catalog context stay independent", async ({
  page,
}, testInfo) => {
  test.setTimeout(90000);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await login(page);
  const a = cards(page).first();
  await scope(page, a);
  await metric(page, a);
  await run(a);
  await a.getByRole("button", { name: "复制查询", exact: true }).click();
  const b = cards(page).nth(1);
  await scope(page, b, false, 1);
  await run(b);
  await expect(
    a.getByRole("status").filter({ hasText: "查询完成" }),
  ).toBeVisible();
  await a.getByRole("button", { name: /资源范围 ·/ }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "清空选择", exact: true }).click();
  await dialog.getByRole("button", { name: /确认选择/ }).click();
  await expect(
    a.getByRole("button", { name: "执行查询", exact: true }),
  ).toBeDisabled();
  await expect(a.getByRole("img")).toHaveCount(0);
  await expect(b.getByRole("img")).toBeVisible();
  await page.getByRole("tab", { name: "指标与字段目录", exact: true }).click();
  const catalog = page.locator(".argus-observe-catalog");
  await scope(page, catalog);
  const resource = catalog.getByRole("button", { name: /资源范围 ·/ });
  await resource.click();
  await dialog
    .getByRole("button", { name: /^选择 / })
    .nth(1)
    .click();
  await dialog.getByRole("button", { name: /确认选择/ }).click();
  await expect(resource).toContainText("已选 2 项");
  await page.getByPlaceholder("搜索指标名称或字段…").fill("system_cpu");
  await expect(
    page.getByRole("button", { name: "system_cpu_utilization", exact: true }),
  ).toBeVisible();
  await select(page, page, "资源类型", "Kubernetes 集群");
  await expect(
    page.getByText("当前筛选没有匹配资源", { exact: true }),
  ).toBeVisible();
  await select(page, page, "资源类型", "全部资源类型");
  await expect(
    page.getByRole("button", { name: "system_cpu_utilization", exact: true }),
  ).toBeVisible();
  const resourceBox = await resource.boundingBox();
  const sourceBox = await selectTrigger(page, "采集来源").boundingBox();
  expect(resourceBox).not.toBeNull();
  expect(sourceBox).not.toBeNull();
  expect(Math.abs(resourceBox!.y - sourceBox!.y)).toBeLessThan(10);
  await expect(
    catalog
      .locator(".argus-filter-bar")
      .getByRole("button", { name: "时间范围", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("catalog-desktop.png"),
    fullPage: true,
  });
  await page
    .getByRole("row")
    .filter({
      has: page.getByRole("button", {
        name: "system_cpu_utilization",
        exact: true,
      }),
    })
    .getByRole("button", { name: "新建查询", exact: true })
    .click();
  await expect(cards(page)).toHaveCount(3);
  await expect(b.getByRole("img")).toBeVisible();
  await expect(
    cards(page)
      .last()
      .getByRole("button", { name: /资源范围 · 已选 2 项/ }),
  ).toBeVisible();
});

test("catalog pagination resets with time and search, and empty scope never becomes all resources", async ({
  page,
}) => {
  await login(page);
  await page.getByRole("tab", { name: "指标与字段目录", exact: true }).click();
  const catalog = page.locator(".argus-observe-catalog");
  await scope(page, catalog);
  await catalog.getByRole("tab", { name: "日志", exact: true }).click();
  await catalog.getByRole("button", { name: "下一页", exact: true }).click();
  await expect(
    catalog.getByRole("button", { name: "第 2 页", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await catalog.getByRole("button", { name: "时间范围", exact: true }).click();
  const timePicker = page.getByRole("dialog", {
    name: "时间范围",
    exact: true,
  });
  await select(page, timePicker, "时间范围类型", "相对时间");
  await select(page, timePicker, "常用范围", "最近 6 小时");
  await timePicker.getByRole("button", { name: "应用", exact: true }).click();
  await expect(
    catalog.getByRole("button", { name: "第 1 页", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await expect(catalog.getByRole("alert")).toHaveCount(0);
  await catalog.getByRole("button", { name: "下一页", exact: true }).click();
  await catalog.getByPlaceholder("搜索指标名称或字段…").fill("field_29");
  await expect(
    catalog.getByRole("cell", {
      name: "structured_metadata.field_29",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    catalog.getByRole("button", { name: "第 1 页", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await catalog.getByRole("button", { name: /资源范围 ·/ }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "清空选择", exact: true }).click();
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(
    catalog.getByRole("cell", {
      name: "structured_metadata.field_29",
      exact: true,
    }),
  ).toBeVisible();
  await catalog.getByRole("button", { name: /资源范围 ·/ }).click();
  await dialog.getByRole("button", { name: "清空选择", exact: true }).click();
  await dialog.getByRole("button", { name: /确认选择/ }).click();
  await expect(catalog.getByRole("table")).toHaveCount(0);
  await expect(
    catalog.getByRole("button", { name: /资源范围 · 已选 0 项/ }),
  ).toBeVisible();
});

test("language completion includes labels, values, KQL stages and read-only GraphQL fields", async ({
  page,
}) => {
  test.setTimeout(90000);
  await login(page);
  const card = cards(page).first();
  await scope(page, card);
  await metric(page, card);
  await setChoice(card.getByRole("radio", { name: "DSL 编辑器", exact: true }));
  await expect(card.locator(".cm-editor")).toBeVisible();
  let code = card.getByRole("textbox", { name: "查询语句", exact: true });
  await code.fill("system_cpu_utilization{st");
  await code.press("Control+Space");
  await expect(
    page.getByRole("option", { name: /^state/ }).first(),
  ).toBeVisible();
  await code.press("Escape");
  await code.fill('system_cpu_utilization{state="i');
  await code.press("Control+Space");
  await expect(
    page.getByRole("option", { name: /^idle/ }).first(),
  ).toBeVisible();
  await code.press("Escape");
  await select(page, card, "查询类型", "日志");
  await setChoice(card.getByRole("radio", { name: "DSL 编辑器", exact: true }));
  const pipeline = card.getByRole("textbox", { name: "处理管道", exact: true });
  await expect(card.locator(".cm-editor")).toHaveCount(2);
  await pipeline.fill("st");
  await pipeline.press("Control+Space");
  await expect(
    page.getByRole("option", { name: /^stats/ }).first(),
  ).toBeVisible();
  await pipeline.press("Escape");
  await select(page, card, "查询类型", "链路");
  await setChoice(card.getByRole("radio", { name: "DSL 编辑器", exact: true }));
  await expect(card.locator(".cm-editor")).toHaveCount(1);
  code = card.getByRole("textbox", { name: "查询语句", exact: true });
  await code.fill("{ queryTr");
  await code.press("Control+Space");
  await expect(
    page.getByRole("option", { name: /^queryTraces/ }).first(),
  ).toBeVisible();
});

test("empty query mode switching and invalid GraphQL variables never execute stale input", async ({
  page,
}) => {
  test.setTimeout(90000);
  await login(page);
  const card = cards(page).first();
  await setChoice(card.getByRole("radio", { name: "DSL 编辑器", exact: true }));
  await expect(card.locator(".cm-editor")).toBeVisible();
  await expect(
    card.getByRole("button", { name: "执行查询", exact: true }),
  ).toBeDisabled();
  await setChoice(card.getByRole("radio", { name: "查询构建器", exact: true }));
  await select(page, card, "查询类型", "链路");
  await scope(page, card);
  await setChoice(card.getByRole("radio", { name: "DSL 编辑器", exact: true }));
  await expect(card.locator(".cm-editor")).toBeVisible();
  await card.getByText("高级查询选项", { exact: true }).click();
  const variables = card.getByRole("textbox", {
    name: "GraphQL 常量变量（JSON）",
    exact: true,
  });
  await variables.fill("{invalid");
  await card.locator(".argus-observe-query-heading strong").click();
  await expect(card.locator(".argus-card__content")).toBeHidden();
  await page.getByRole("button", { name: "执行全部", exact: true }).click();
  await expect(card.locator(".argus-card__content")).toBeVisible();
  await expect(
    card.getByText("请修正无效的查询字段，尚未执行查询。", { exact: true }),
  ).toBeVisible();
  await expect(variables).toHaveValue("{invalid");
  await expect(
    card.getByRole("columnheader", { name: "调用链 ID", exact: true }),
  ).toHaveCount(0);
});

test("absolute time picker applies historical ranges and cancel retains existing results", async ({
  page,
}, info) => {
  await login(page);
  const card = cards(page).first();
  await scope(page, card);
  await metric(page, card);
  await run(card);
  const time = page
    .locator(".argus-observe-global")
    .getByRole("button", { name: "时间范围", exact: true });
  const original = await time.innerText();
  await time.click();
  const picker = page.getByRole("dialog", { name: "时间范围", exact: true });
  await expect(selectTrigger(picker, "时间范围类型")).toContainText(
    "固定时间范围",
  );
  const year = picker.getByRole("spinbutton", { name: /年, 开始时间/ });
  await year.focus();
  await year.pressSequentially("2000");
  await picker.getByRole("button", { name: "取消", exact: true }).click();
  await expect(time).toHaveText(original!);
  await expect(card.getByRole("img")).toBeVisible();
  await time.click();
  for (const name of ["开始时间", "结束时间"]) {
    const year = picker.getByRole("spinbutton", {
      name: new RegExp(`年, ${name}`),
    });
    await year.focus();
    await year.pressSequentially("2000");
  }
  await picker.getByRole("button", { name: "应用", exact: true }).click();
  await expect(time).toContainText("2000-");
  await expect(card.getByRole("img")).toHaveCount(0);
  await run(card);
  await expect(
    card.locator("small.argus-observe-hint").filter({ hasText: /2000/ }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("absolute-query.png"),
    animations: "disabled",
  });
});
