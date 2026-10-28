import { expect, test, type Page } from "@playwright/test";

test.use({
  viewport: { width: 1440, height: 900 },
  actionTimeout: 10000,
  contextOptions: { reducedMotion: "reduce" },
});
async function bottom(page: Page) {
  await page.locator(".argus-combobox__popover").evaluate(async (el) => {
    await Promise.all(
      el
        .getAnimations({ subtree: true })
        .filter((a) => Number.isFinite(a.effect?.getComputedTiming().endTime))
        .map((a) => a.finished.catch(() => undefined)),
    );
  });
  await page.getByRole("listbox").hover();
  await page.mouse.wheel(0, 4000);
}
const options = (start: number, end: number, prefix = "metric") =>
  Array.from({ length: end - start }, (_, i) => ({
    value: `${prefix}_${start + i}`,
    label: `${prefix}_${start + i}`,
  }));

for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"])
    test(`${locale} ${theme}: wheel loads each next page once and keeps selection`, async ({
      page,
    }) => {
      const requests: string[] = [];
      await page.route("**/pagination-fixture?**", async (route) => {
        const cursor = new URL(route.request().url()).searchParams.get(
          "cursor",
        )!;
        requests.push(cursor);
        const start = Number(cursor || 0);
        await route.fulfill({
          json: {
            options: options(start, Math.min(start + 40, 95)),
            next_cursor: start < 80 ? String(start + 40) : undefined,
          },
        });
      });
      await page.goto(
        `/e2e/fixtures/combobox-pagination.html?locale=${locale}&theme=${theme}`,
      );
      await expect.poll(() => requests.length).toBe(1);
      await page.getByRole("combobox", { name: "Metric", exact: true }).click();
      await page
        .getByRole("combobox", { name: "Metric", exact: true })
        .press("ArrowDown");
      await expect(
        page.getByRole("option", { name: "metric_39", exact: true }),
      ).toBeAttached();
      await expect(
        page.getByRole("button", { name: /加载更多|Load more/ }),
      ).toHaveCount(0);
      await bottom(page);
      await expect(
        page.getByRole("option", { name: "metric_79", exact: true }),
      ).toBeAttached();
      expect(requests).toEqual(["", "40"]);
      await bottom(page);
      await expect(
        page.getByRole("option", { name: "metric_94", exact: true }),
      ).toBeAttached();
      await bottom(page);
      expect(requests).toEqual(["", "40", "80"]);
      await expect(page.getByRole("option")).toHaveCount(96);
      await expect(page.getByLabel("Selected metric")).toHaveText("selected");
      await page
        .getByRole("option", { name: "metric_94", exact: true })
        .click();
      await expect(page.getByLabel("Selected metric")).toHaveText("metric_94");
    });

test("failed next page keeps prior pages and retries that cursor without a loop", async ({
  page,
}) => {
  const requests: string[] = [];
  await page.route("**/pagination-fixture?**", async (route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor")!;
    requests.push(cursor);
    if (requests.length === 2)
      return route.fulfill({ status: 503, body: "unavailable" });
    await route.fulfill({
      json: {
        options: options(Number(cursor || 0), cursor ? 60 : 40),
        next_cursor: cursor ? undefined : "40",
      },
    });
  });
  await page.goto("/e2e/fixtures/combobox-pagination.html");
  await page
    .getByRole("combobox", { name: "Metric", exact: true })
    .press("ArrowDown");
  await expect(
    page.getByRole("option", { name: "metric_39", exact: true }),
  ).toBeAttached();
  await bottom(page);
  await expect(page.getByRole("alert")).toBeVisible();
  await bottom(page);
  await expect(page.getByRole("option")).toHaveCount(41);
  expect(requests).toEqual(["", "40"]);
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await expect(
    page.getByRole("option", { name: "metric_59", exact: true }),
  ).toBeAttached();
  expect(requests).toEqual(["", "40", "40"]);
  await expect(page.getByRole("alert")).toHaveCount(0);
});

test("a late page cannot overwrite a new search or scope", async ({ page }) => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const requests: URL[] = [];
  await page.route("**/pagination-fixture?**", async (route) => {
    const url = new URL(route.request().url());
    requests.push(url);
    const cursor = url.searchParams.get("cursor"),
      search = url.searchParams.get("search"),
      scope = url.searchParams.get("scope");
    if (cursor) await gate;
    await route
      .fulfill({
        json:
          search === "new" || scope === "b"
            ? { options: options(0, 2, `${scope}_new`) }
            : {
                options: options(cursor ? 40 : 0, cursor ? 60 : 40),
                next_cursor: cursor ? undefined : "40",
              },
      })
      .catch(() => undefined);
  });
  await page.goto("/e2e/fixtures/combobox-pagination.html");
  const input = page.getByRole("combobox", { name: "Metric", exact: true });
  await input.press("ArrowDown");
  await expect(
    page.getByRole("option", { name: "metric_39", exact: true }),
  ).toBeAttached();
  await bottom(page);
  await expect.poll(() => requests.length).toBe(2);
  await bottom(page);
  expect(requests.length).toBe(2);
  await input.fill("new");
  await expect(
    page.getByRole("option", { name: "a_new_1", exact: true }),
  ).toBeAttached();
  release();
  await expect(
    page.getByRole("option", { name: "metric_59", exact: true }),
  ).toHaveCount(0);
  await page.getByText("Change scope", { exact: true }).click();
  await input.press("ArrowDown");
  await expect(
    page.getByRole("option", { name: "b_new_1", exact: true }),
  ).toBeAttached();
  await expect(
    page.getByRole("option", { name: "a_new_1", exact: true }),
  ).toHaveCount(0);
  expect(requests.at(-1)!.searchParams.get("cursor")).toBe("");
});

test("custom metric search stays open after scrolling to another page", async ({
  page,
}) => {
  await page.route("**/pagination-fixture?**", async (route) => {
    const params = new URL(route.request().url()).searchParams;
    const cursor = params.get("cursor"),
      search = params.get("search");
    await route.fulfill({
      json:
        search === "new"
          ? { options: options(0, 2, "new") }
          : {
              options: options(cursor ? 40 : 0, cursor ? 60 : 40),
              next_cursor: cursor ? undefined : "40",
            },
    });
  });
  await page.goto("/e2e/fixtures/combobox-pagination.html?custom=1");
  const input = page.getByRole("combobox", { name: "Metric", exact: true });
  await input.press("ArrowDown");
  await expect(
    page.getByRole("option", { name: "metric_39", exact: true }),
  ).toBeAttached();
  await bottom(page);
  await expect(
    page.getByRole("option", { name: "metric_59", exact: true }),
  ).toBeAttached();
  await input.fill("new");
  await expect(
    page.getByRole("option", { name: "new_1", exact: true }),
  ).toBeVisible();
});

test("short pages fill the popup automatically and static options remain selectable", async ({
  page,
}) => {
  const cursors: string[] = [];
  await page.route("**/pagination-fixture?**", async (route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor")!;
    cursors.push(cursor);
    const start = Number(cursor || 0);
    await route.fulfill({
      json: {
        options: options(start, start + 2),
        next_cursor: start < 4 ? String(start + 2) : undefined,
      },
    });
  });
  await page.goto("/e2e/fixtures/combobox-pagination.html");
  const input = page.getByRole("combobox", { name: "Metric", exact: true });
  await input.press("ArrowDown");
  await expect(
    page.getByRole("option", { name: "metric_5", exact: true }),
  ).toBeAttached();
  expect(cursors).toEqual(["", "2", "4"]);
  await page.getByText("Change scope", { exact: true }).click();
  const fixed = page.getByRole("combobox", { name: "Static", exact: true });
  await fixed.fill("Dog");
  await expect(
    page.getByRole("option", { name: "Dog", exact: true }),
  ).toBeVisible();
  await page.getByRole("option", { name: "Dog", exact: true }).click();
  await expect(fixed).toHaveValue("dog");
});
