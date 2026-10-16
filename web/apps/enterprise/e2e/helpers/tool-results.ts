import type { Page } from "@playwright/test";

/** Rendering assertions explicitly opt in to the otherwise lazy tool results. */
export async function openToolResults(page: Page) {
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
