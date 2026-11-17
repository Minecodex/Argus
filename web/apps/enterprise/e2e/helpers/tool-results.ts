import type { Page } from "@playwright/test";

export async function openActivityGroups(page: Page) {
  await page
    .locator(
      '.argus-tool-activity-group, .argus-tool-activity, [data-testid="tool-result"]',
    )
    .first()
    .waitFor();
  const groups = page.locator(
    '.argus-tool-activity-group > button[aria-expanded="false"]',
  );
  for (let opened = 0; await groups.count(); opened++) {
    if (opened > 500) throw new Error("Activity groups did not settle");
    await groups.first().click();
  }
}

/** Rendering assertions explicitly opt in to the otherwise lazy tool results. */
export async function openToolResults(page: Page) {
  const activities = page.locator(
    ".argus-tool-activity-group, .argus-tool-activity",
  );
  await page
    .locator(
      ".argus-tool-activity-group, .argus-tool-activity, .argus-tool-result__header button",
    )
    .first()
    .waitFor();
  if (await activities.count()) {
    const groups = page.locator(
      '.argus-tool-activity-group > button[aria-expanded="false"]',
    );
    for (let opened = 0; await groups.count(); opened++) {
      if (opened > 500) throw new Error("Activity groups did not settle");
      await groups.first().click();
    }
    const rows = page.locator(
      '.argus-tool-activity > button[aria-expanded="false"]',
    );
    for (let opened = 0; await rows.count(); opened++) {
      if (opened > 500) throw new Error("Tool activities did not settle");
      await rows.first().click();
    }
    const views = page.locator(
      'details[data-testid="tool-business-view"]:not([open]) > summary',
    );
    for (let opened = 0; await views.count(); opened++) {
      if (opened > 500) throw new Error("Business views did not settle");
      await views.first().click();
    }
    return;
  }
  const buttons = page.locator(".argus-tool-result__header button");
  await buttons.first().waitFor();
  const closed = page.locator(
    '.argus-tool-result__header button[aria-expanded="false"]',
  );
  // SSE/history reconciliation may reorder activities while a large conversation
  // loads. Resolve the next closed control afresh instead of retaining nth locators.
  for (let opened = 0; await closed.count(); opened++) {
    if (opened > 500) throw new Error("Tool results did not settle");
    await closed.first().click();
  }
}
