import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import { selectRowAction } from "./helpers/action-menu";
import {
  kubernetesResourcesZh,
  kubernetesResourcesEn,
} from "../src/i18n/kubernetes-resources";
import {
  kubernetesWorkbenchZh,
  kubernetesWorkbenchEn,
} from "../src/i18n/kubernetes-workbench";
const root = process.env.ARGUS_KWR_ARTIFACTS ?? "";
const mapping = root
  ? JSON.parse(readFileSync(root + "/lab-resources.json", "utf8"))
  : {};
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
test.describe.configure({ mode: "serial" });
test.skip(
  process.env.ARGUS_KWR_REAL !== "1",
  "Requires an owned actual ConfigMap",
);
for (const english of [false, true])
  for (const theme of ["light", "dark"])
    test(`multiline annotations preserve files and aligned controls ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(90000);
      page.setDefaultTimeout(20000);
      await page.setViewportSize({ width: 1440, height: 1080 });
      const r = (english ? kubernetesResourcesEn : kubernetesResourcesZh)
          .kubernetesResources,
        w = (english ? kubernetesWorkbenchEn : kubernetesWorkbenchZh)
          .kubernetesWorkbench;
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.goto("/login");
      await page.locator('input[autocomplete="username"]').fill("kakj");
      await page
        .locator('input[autocomplete="current-password"]')
        .fill(process.env.ARGUS_KWR_PASSWORD!);
      await page.locator('form button[type="submit"]').click();
      await expect(page).not.toHaveURL(/\/login/);
      const url = `/kubernetes/${mapping.cluster}?tab=resources&kind=configmap&namespace_id=${mapping.namespace}&namespace_name=kwr-workloads`;
      const row = page.getByRole("row").filter({
        has: page.getByRole("button", { name: "kwr-config", exact: true }),
      });
      await page.goto(url);
      await selectRowAction(row, r.actions.configure);
      let dialog = page.getByRole("dialog", {
        name: r.actions.configure + " · kwr-config",
        exact: true,
      });
      const before = await dialog
        .getByRole("group", { name: r.data, exact: true })
        .locator("textarea")
        .first()
        .inputValue();
      const annotations = dialog.getByRole("group", {
        name: r.annotations,
        exact: true,
      });
      const entry = annotations
        .locator(".argus-key-value-editor__row")
        .filter({ has: page.locator('input[value="kwr.multiline"]') });
      if (!(await entry.count())) {
        await annotations
          .getByRole("button", {
            name: english ? "Add entry" : "添加一项",
            exact: true,
          })
          .click();
        await annotations
          .locator(".argus-key-value-editor__row")
          .last()
          .getByRole("textbox", { name: english ? /^Key / : /^键 / })
          .fill("kwr.multiline");
      }
      const note = `First line\n第二行\n${english ? "en" : "zh"}-${theme}-${Date.now()}\n`;
      await entry.locator("textarea").fill(note);
      const keyBox = await entry.locator("input").boundingBox(),
        valueBox = await entry.locator("textarea").boundingBox(),
        removeBox = await entry.getByRole("button").boundingBox();
      expect(Math.abs(keyBox!.y - valueBox!.y)).toBeLessThan(2);
      expect(Math.abs(removeBox!.y - valueBox!.y)).toBeLessThan(2);
      await page.screenshot({
        path: info.outputPath("aligned-multiline-form.png"),
        fullPage: true,
      });
      await dialog
        .getByRole("button", { name: w.mutation.preview, exact: true })
        .click();
      await dialog
        .getByRole("button", { name: w.mutation.confirm, exact: true })
        .click();
      await expect(dialog).not.toBeVisible();
      await expect(page.locator(".argus-k8s-operation-feedback")).toContainText(
        w.mutation.statuses.succeeded,
        { timeout: 45000 },
      );
      const read = page.waitForResponse(
        (response) =>
          response.url().includes("/inventory/object?") &&
          response.url().includes("name=kwr-config") &&
          response.status() === 200,
      );
      await selectRowAction(row, r.actions.configure);
      const object = await (await read).json();
      expect(object.data.metadata.annotations["kwr.multiline"]).toBe(note);
      expect(object.data.data["app.yaml"]).toBe(before);
      expect(object.data.data["untouched.txt"]).toBe("retain\n");
      dialog = page.getByRole("dialog", {
        name: r.actions.configure + " · kwr-config",
        exact: true,
      });
      await expect(
        dialog
          .getByRole("group", { name: r.annotations, exact: true })
          .locator("textarea")
          .filter({ hasText: note }),
      ).toHaveValue(note);
      await page.keyboard.press("Escape");
    });
