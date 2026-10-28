import { test, expect, type Locator, type Page } from "@playwright/test";
async function hover(page: Page, chart: Locator) {
  await expect(chart.locator("canvas")).toBeVisible();
  const box = (await chart.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  const tooltip = page
    .locator(".argus-chart-tooltip")
    .filter({ visible: true })
    .last();
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toHaveCSS("opacity", "1");
  expect(
    await tooltip.evaluate(
      (element) => element.parentElement === document.body,
    ),
  ).toBe(true);
  expect(
    Number(
      await tooltip.evaluate((element) => getComputedStyle(element).zIndex),
    ),
  ).toBe(2147483647);
  return tooltip;
}
for (const theme of ["light", "dark"])
  test(`chart tooltips escape clipped cards and modal overlays in ${theme}`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(`/e2e/fixtures/observation-tooltip.html?theme=${theme}`);
    const tooltip = await hover(
      page,
      page.getByRole("img", { name: "Shared chart", exact: true }),
    );
    await expect(tooltip).toContainText("<img src=x");
    await expect(tooltip.locator("img")).toHaveCount(0);
    expect(
      await page.evaluate(
        () =>
          (window as Window & { tooltipInjected?: boolean }).tooltipInjected,
      ),
    ).toBeUndefined();
    const bounds = (await tooltip.boundingBox())!,
      card = (await page.getByTestId("clipped-chart").boundingBox())!;
    expect(bounds.y + bounds.height).toBeGreaterThan(card.y + card.height);
    await page.screenshot({
      path: info.outputPath("tooltip-above-card.png"),
      animations: "disabled",
    });
    await hover(
      page,
      page.getByRole("img", { name: "Legacy chart", exact: true }),
    );
    await page
      .getByRole("button", { name: "Open chart dialog", exact: true })
      .click();
    await hover(
      page,
      page.getByRole("img", { name: "Dialog chart", exact: true }),
    );
    await page.screenshot({
      path: info.outputPath("tooltip-above-dialog.png"),
      animations: "disabled",
    });
  });
