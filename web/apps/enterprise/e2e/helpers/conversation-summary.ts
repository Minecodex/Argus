import type { Page } from "@playwright/test";

export async function openSummarySection(
  page: Page,
  section: "mcp" | "files" | "resources",
) {
  const content = page.getByTestId(`conversation-${section}-section`);
  await content.waitFor({ state: "visible" });
  if ((await content.getAttribute("open")) === null)
    await content.locator(":scope > summary").click();
}
