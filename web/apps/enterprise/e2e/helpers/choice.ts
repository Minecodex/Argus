import { expect, type Locator } from "@playwright/test";

/** React Aria exposes a focusable input behind its visible control. */
export async function setChoice(choice: Locator, selected = true) {
  if ((await choice.isChecked()) !== selected)
    await choice.press("Space", { timeout: 15000 });
  if (selected) await expect(choice).toBeChecked();
  else await expect(choice).not.toBeChecked();
}
