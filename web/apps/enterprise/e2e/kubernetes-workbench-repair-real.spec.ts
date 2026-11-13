import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { expect, test, type Page, type Locator } from "@playwright/test";
import { selectRowAction } from "./helpers/action-menu";
import {
  kubernetesResourcesZh,
  kubernetesResourcesEn,
} from "../src/i18n/kubernetes-resources";
import {
  kubernetesWorkbenchZh,
  kubernetesWorkbenchEn,
} from "../src/i18n/kubernetes-workbench";
import {
  kubernetesStreamZh,
  kubernetesStreamEn,
} from "../src/i18n/kubernetes-stream";
import {
  kubernetesLogSessionZh,
  kubernetesLogSessionEn,
} from "../src/i18n/kubernetes-log-session";

const root = process.env.ARGUS_KWR_ARTIFACTS ?? "";
const mapping = root
  ? JSON.parse(readFileSync(root + "/lab-resources.json", "utf8"))
  : {};
const pvc =
  root && process.env.ARGUS_KWR_REAL === "1"
    ? JSON.parse(readFileSync(root + "/pvc-fixture.json", "utf8"))
    : {};
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
test.describe.configure({ mode: "serial" });
test.skip(
  process.env.ARGUS_KWR_REAL !== "1",
  "Requires explicitly owned actual resources",
);
async function login(
  page: Page,
  english: boolean,
  theme: string,
  reader = false,
) {
  page.setDefaultTimeout(20000);
  await page.setViewportSize({ width: 1440, height: 1080 });
  await page.addInitScript(
    ({ english, theme }) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  const credentials = reader
    ? JSON.parse(readFileSync(root + "/private/reader-login.json", "utf8"))
    : { username: "kakj", password: process.env.ARGUS_KWR_PASSWORD! };
  await page.goto("/login");
  await page
    .locator('input[autocomplete="username"]')
    .fill(credentials.username);
  await page
    .locator('input[autocomplete="current-password"]')
    .fill(credentials.password);
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}
async function list(
  page: Page,
  kind: string,
  cluster = mapping.cluster,
  namespace = mapping.namespace,
) {
  await page.goto(
    `/kubernetes/${cluster}?tab=resources&kind=${encodeURIComponent(kind)}&namespace_id=${namespace}&namespace_name=${cluster === mapping.cluster ? "kwr-workloads" : pvc.namespace_name}`,
  );
  await expect(page.locator(".argus-k8s-workbench")).toBeVisible();
}
const resourceRow = (page: Page, name: string | RegExp) =>
  page
    .getByRole("row")
    .filter({
      has: page.getByRole("button", { name, exact: typeof name === "string" }),
    })
    .first();

let index = 0;
for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    const turn = index++;
    test(`actual repairs and mutations ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(360000);
      const r = (english ? kubernetesResourcesEn : kubernetesResourcesZh)
        .kubernetesResources;
      const w = (english ? kubernetesWorkbenchEn : kubernetesWorkbenchZh)
        .kubernetesWorkbench;
      const stream = (english ? kubernetesStreamEn : kubernetesStreamZh)
        .kubernetesStream;
      const logs = (english ? kubernetesLogSessionEn : kubernetesLogSessionZh)
        .kubernetesLogSession;
      const errors: string[] = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await login(page, english, theme);
      const settle = async (dialog: Locator) => {
        const preview = dialog.getByRole("button", {
          name: w.mutation.preview,
          exact: true,
        });
        const confirm = dialog.getByRole("button", {
          name: w.mutation.confirm,
          exact: true,
        });
        const rebase = dialog.getByRole("button", {
          name: r.rebase,
          exact: true,
        });
        for (let attempt = 0; attempt < 3; attempt++) {
          if (await rebase.isVisible()) await rebase.click();
          await preview.click();
          await expect(confirm.or(rebase)).toBeVisible();
          if (await confirm.isVisible()) break;
        }
        await confirm.click();
        await expect(dialog).not.toBeVisible();
        await expect(
          page.locator(".argus-k8s-operation-feedback"),
        ).toContainText(w.mutation.statuses.succeeded, { timeout: 120000 });
      };
      await list(page, "deployment");
      await expect(resourceRow(page, "kwr-api")).toBeVisible();
      const refresh = page.waitForResponse(
        (response) =>
          response.url().includes("/inventory?") &&
          response.request().method() === "GET" &&
          response.status() === 200,
      );
      await page
        .locator(".argus-k8s-workbench")
        .getByRole("button", { name: w.refresh, exact: true })
        .click();
      await refresh;
      const row = resourceRow(page, "kwr-api");
      await selectRowAction(row, r.actions.scale);
      let dialog = page.getByRole("dialog", {
        name: r.actions.scale + " · kwr-api",
        exact: true,
      });
      const replicas =
        Number(
          await dialog
            .getByRole("spinbutton", { name: w.mutation.replicas })
            .inputValue(),
        ) === 2
          ? 1
          : 2;
      await dialog
        .getByRole("spinbutton", { name: w.mutation.replicas })
        .fill(String(replicas));
      await settle(dialog);
      await expect(row).toContainText(`${replicas} / ${replicas}`);
      console.log("Actual scale applied", replicas);
      await selectRowAction(row, r.actions.image);
      dialog = page.getByRole("dialog", {
        name: r.actions.image + " · kwr-api",
        exact: true,
      });
      await page.screenshot({
        path: info.outputPath("image-form.png"),
        fullPage: true,
      });
      const imageField = dialog.getByRole("textbox", {
        name: "main",
        exact: true,
      });
      const image =
        (await imageField.inputValue()) === "busybox:1.37"
          ? "busybox:1.36"
          : "busybox:1.37";
      await imageField.fill(image);
      await settle(dialog);
      await expect(row).toContainText(image);
      console.log("Actual image rollout applied", image);

      await list(page, "configmap");
      await selectRowAction(
        resourceRow(page, "kwr-config"),
        r.actions.configure,
      );
      dialog = page.getByRole("dialog", {
        name: r.actions.configure + " · kwr-config",
        exact: true,
      });
      const content = dialog
        .getByRole("group", { name: r.data, exact: true })
        .getByRole("textbox", {
          name: english ? "Value 1" : "值 1",
          exact: true,
        });
      await expect(content).toHaveValue(/\n/);
      expect(await content.evaluate((node) => node.tagName)).toBe("TEXTAREA");
      const file = `server:\n  port: ${8081 + turn}\n  mode: ${english ? "en" : "zh"}-${theme}\n  iteration: ${Date.now()}\n\n`;
      await content.fill(file);
      if (turn === 0) {
        const native = [
          "--kubeconfig",
          root + "/private/drain-kubeconfig.yaml",
          "--context",
          "kind-plan8-drain-kwr-20261112-r2",
          "-n",
          "kwr-workloads",
        ];
        const current = JSON.parse(
          execFileSync(
            "kubectl",
            [...native, "get", "configmap", "kwr-config", "-o", "json"],
            { encoding: "utf8" },
          ),
        );
        expect(current.metadata.labels["argus.owner"]).toBe("kwr-20261112-r2");
        execFileSync(
          "kubectl",
          [
            ...native,
            "annotate",
            "configmap",
            "kwr-config",
            "kwr.concurrent=preserved-" + Date.now(),
            "--overwrite",
          ],
          { encoding: "utf8" },
        );
        await dialog
          .getByRole("button", { name: w.mutation.preview, exact: true })
          .click();
        await expect(
          dialog.getByRole("button", { name: r.rebase, exact: true }),
        ).toBeVisible();
        await expect(content).toHaveValue(file);
        await dialog
          .getByRole("button", { name: r.rebase, exact: true })
          .click();
        await expect(content).toHaveValue(file);
      }
      await settle(dialog);
      console.log("Multiline configuration applied without losing input");
      await page
        .locator(".argus-k8s-operation-feedback")
        .getByRole("button", { name: r.operationDetails, exact: true })
        .click();
      const history = page.getByRole("dialog", {
        name: w.mutation.history,
        exact: true,
      });
      await expect(
        history.getByRole("heading", { name: "kwr-config", exact: true }),
      ).toBeVisible();
      await page.keyboard.press("Escape");
      await selectRowAction(
        resourceRow(page, "kwr-config"),
        r.actions.configure,
      );
      dialog = page.getByRole("dialog", {
        name: r.actions.configure + " · kwr-config",
        exact: true,
      });
      await expect(
        dialog
          .getByRole("group", { name: r.data, exact: true })
          .getByRole("textbox", {
            name: english ? "Value 1" : "值 1",
            exact: true,
          }),
      ).toHaveValue(file);
      await expect(
        dialog
          .getByRole("group", { name: r.data, exact: true })
          .getByRole("textbox", {
            name: english ? "Value 2" : "值 2",
            exact: true,
          }),
      ).toHaveValue("retain\n");
      await page.screenshot({
        path: info.outputPath("multiline-config.png"),
        fullPage: true,
      });
      await page.keyboard.press("Escape");

      await list(page, "cronjob");
      await selectRowAction(resourceRow(page, "kwr-cron"), r.actions.run);
      dialog = page.getByRole("dialog", {
        name: w.jobRun.title.replace("{{name}}", "kwr-cron"),
        exact: true,
      });
      const job = `kwr-r2-job-${turn}-${Date.now().toString(36)}`;
      await dialog
        .getByRole("textbox", { name: w.jobRun.name, exact: true })
        .fill(job);
      await settle(dialog);
      await page
        .locator(".argus-k8s-operation-feedback")
        .getByRole("button", { name: r.openResult, exact: true })
        .click();
      await expect(
        page.getByRole("dialog", { name: "Job · " + job, exact: true }),
      ).toBeVisible();
      await expect(page).toHaveURL(/resource_uid=/);
      await page.keyboard.press("Escape");

      await list(page, "pod");
      const pod = resourceRow(page, /^kwr-api-/);
      await selectRowAction(pod, r.actions.logs);
      dialog = page.getByRole("dialog", { name: /^Pod (日志|logs)/i });
      await expect(dialog.getByText(/kwr-actual-log/).first()).toBeVisible({
        timeout: 30000,
      });
      await dialog
        .getByRole("button", { name: logs.options, exact: true })
        .click();
      const boxes = await dialog
        .getByRole("spinbutton")
        .evaluateAll((nodes) =>
          nodes.map((node) => node.getBoundingClientRect().top),
        );
      expect(Math.max(...boxes) - Math.min(...boxes)).toBeLessThan(2);
      await page.screenshot({
        path: info.outputPath("actual-logs.png"),
        fullPage: true,
      });
      await dialog
        .getByRole("button", { name: logs.close, exact: true })
        .click();
      const output: string[] = [];
      page.on("websocket", (socket) =>
        socket.on("framereceived", (event) => {
          try {
            const frame = JSON.parse(String(event.payload));
            if (frame.type === "output")
              output.push(Buffer.from(frame.data, "base64").toString());
          } catch {
            // Non-JSON frames do not contain terminal output.
          }
        }),
      );
      await selectRowAction(pod, r.actions.exec);
      dialog = page.getByRole("dialog", { name: /^Pod (终端|terminal)/i });
      await dialog
        .getByRole("textbox", { name: stream.reason })
        .fill("Owned KWR R2 real shell verification");
      await dialog
        .getByRole("button", { name: stream.open, exact: true })
        .click();
      await expect(
        dialog.getByText(english ? "Connected" : "已连接", { exact: false }),
      ).toBeVisible({ timeout: 30000 });
      await expect(dialog.locator(".xterm-helper-textarea")).toBeAttached({
        timeout: 30000,
      });
      await dialog.locator(".xterm-helper-textarea").focus();
      await page.keyboard.type("printf 'KWR_R2_TERMINAL_OK\\n'; id; pwd");
      await page.keyboard.press("Enter");
      await expect
        .poll(() => output.join(""), { timeout: 30000 })
        .toContain("KWR_R2_TERMINAL_OK");
      await expect.poll(() => output.join("")).toMatch(/uid=\d+/);
      await page.screenshot({
        path: info.outputPath("actual-terminal.png"),
        fullPage: true,
      });
      await dialog
        .getByRole("button", { name: stream.close, exact: true })
        .click();

      await list(page, "pvc", pvc.cluster, pvc.namespace);
      await selectRowAction(resourceRow(page, pvc.claim), r.actions.expand);
      dialog = page.getByRole("dialog", {
        name: r.actions.expand + " · " + pvc.claim,
        exact: true,
      });
      const capacityField = dialog.getByRole("textbox", {
        name: r.targetCapacity,
      });
      const previous = await capacityField.inputValue();
      expect(previous).toMatch(/^\d+Mi$/);
      const capacity = `${Number(previous.replace("Mi", "")) + 32}Mi`;
      await capacityField.fill(capacity);
      await settle(dialog);
      await expect(resourceRow(page, pvc.claim)).toContainText(capacity);
      const proof = execFileSync(
        "kubectl",
        [
          "--kubeconfig",
          process.env.ARGUS_KWR_B10B_KUBECONFIG!,
          "--context",
          "kind-p9-b10b-20261112-01",
          "-n",
          pvc.namespace_name,
          "exec",
          pvc.pod,
          "--",
          "cat",
          "/data/proof",
        ],
        { encoding: "utf8" },
      );
      expect(proof.trim()).toBe("kwr-volume-proof");
      await page.screenshot({
        path: info.outputPath("actual-expanded-pvc.png"),
        fullPage: true,
      });
      expect(errors).toEqual([]);
    });
  }
