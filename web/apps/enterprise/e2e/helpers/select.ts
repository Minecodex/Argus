import type { Locator, Page } from "@playwright/test";
export function selectTrigger(scope: Page | Locator, label: string) {
  const escaped = label.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return scope
    .getByRole("button", { name: new RegExp(`${escaped}$`) })
    .and(scope.locator(".argus-select__trigger"));
}
