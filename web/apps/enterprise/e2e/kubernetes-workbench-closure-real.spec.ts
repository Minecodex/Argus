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
const root = process.env.ARGUS_KWR_ARTIFACTS ?? "",
  mapping = root
    ? JSON.parse(readFileSync(root + "/lab-resources.json", "utf8"))
    : {};
const nodes = root
  ? JSON.parse(readFileSync(root + "/drain-ownership.json", "utf8")).nodes
  : [];
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
test.describe.configure({ mode: "serial" });
test.skip(
  process.env.ARGUS_KWR_REAL !== "1",
  "Requires owned actual closure fixtures",
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
  const credential = reader
    ? JSON.parse(readFileSync(root + "/private/reader-login.json", "utf8"))
    : { username: "kakj", password: process.env.ARGUS_KWR_PASSWORD! };
  await page.goto("/login");
  await page
    .locator('input[autocomplete="username"]')
    .fill(credential.username);
  await page
    .locator('input[autocomplete="current-password"]')
    .fill(credential.password);
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}
for (const english of [false, true])
  for (const theme of ["light", "dark"])
    test(`remaining workbench contracts ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(240000);
      const r = (english ? kubernetesResourcesEn : kubernetesResourcesZh)
          .kubernetesResources,
        w = (english ? kubernetesWorkbenchEn : kubernetesWorkbenchZh)
          .kubernetesWorkbench;
      await login(page, english, theme);
      const open = async (
        kind: string,
        name: string,
        namespace: string | undefined = mapping.namespace,
      ) => {
        await page.goto(
          `/kubernetes/${mapping.cluster}?tab=resources&kind=${kind}${namespace ? "&namespace_id=" + namespace + "&namespace_name=" + (namespace === mapping.namespace ? "kwr-workloads" : "kwr-secondary") : ""}`,
        );
        await page.getByRole("button", { name, exact: true }).first().click();
        return page.getByRole("dialog");
      };
      const related = async (drawer: Locator) => {
        let next = page.waitForResponse(
          (response) =>
            response.url().includes("/inventory/related?") &&
            response.status() === 200,
        );
        await drawer
          .getByRole("tab", { name: r.relations, exact: true })
          .click();
        let data = await (await next).json();
        const values: string[] = [];
        for (let round = 0; round < 12; round++) {
          if (data.items.length)
            await expect(
              drawer.getByRole("button", {
                name: data.items[0].name,
                exact: true,
              }),
            ).toBeVisible();
          for (const item of data.items) {
            await expect(
              drawer.getByRole("button", { name: item.name, exact: true }),
            ).toBeVisible();
            values.push(item.name);
          }
          if (!data.next_cursor) return values;
          next = page.waitForResponse(
            (response) =>
              response.url().includes("/inventory/related?") &&
              response.status() === 200,
          );
          await drawer
            .getByRole("button", {
              name: english ? "Next page" : "下一页",
              exact: true,
            })
            .click();
          data = await (await next).json();
        }
        throw new Error("relations failed to terminate within bounded fixture");
      };
      let drawer = await open("daemonset", "kwr-daemon");
      await expect(
        drawer
          .getByRole("term")
          .filter({ hasText: r.fields.desired_replicas })
          .locator("..")
          .getByRole("definition"),
      ).toHaveText("2");
      await page.screenshot({
        path: info.outputPath("daemonset-two-nodes.png"),
        fullPage: true,
      });
      drawer = await open("pod", "kwr-multi-failure");
      await expect(drawer.locator(".argus-k8s-detail-state")).toContainText(
        /CrashLoopBackOff|Error/,
        { timeout: 60000 },
      );
      await expect(
        drawer.getByRole("heading", {
          name: r.facts.containerStates,
          exact: true,
        }),
      ).toBeVisible();
      drawer = await open("ingress", "kwr-ingress");
      await expect(
        drawer.getByRole("cell", { name: "/api", exact: true }),
      ).toBeVisible();
      expect(await related(drawer)).toEqual(
        expect.arrayContaining(["kwr-api", "kwr-tls", "kwr-class"]),
      );
      await page.screenshot({
        path: info.outputPath("ingress-relations.png"),
        fullPage: true,
      });
      drawer = await open("pod", "kwr-projected");
      expect(await related(drawer)).toEqual(
        expect.arrayContaining(["kwr-config", "kwr-tls"]),
      );
      drawer = await open("statefulset", "kwr-ledger");
      expect(await related(drawer)).toContain("kwr-headless");
      drawer = await open("rolebinding", "kwr-cross");
      await expect(
        drawer.getByRole("cell", { name: "cross-account", exact: true }),
      ).toBeVisible();
      expect(await related(drawer)).toEqual(
        expect.arrayContaining(["cross-account", "kwr-read"]),
      );
      drawer = await open(
        "serviceaccount",
        "cross-account",
        mapping.second_namespace,
      );
      expect(await related(drawer)).toContain("kwr-cross");
      drawer = await open("role", "kwr-read");
      await drawer
        .getByRole("tab", { name: r.configuration, exact: true })
        .click();
      await expect(
        drawer.getByRole("heading", { name: r.facts.rules, exact: true }),
      ).toBeVisible();
      await expect(
        drawer.getByRole("cell", { name: "get, list", exact: true }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("rbac-rules.png"),
        fullPage: true,
      });
      drawer = await open("node", nodes[0], undefined);
      await expect(
        drawer.getByRole("heading", { name: r.facts.conditions, exact: true }),
      ).toBeVisible();
      await expect(
        drawer
          .getByRole("definition")
          .filter({ hasText: "control-plane" })
          .first(),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("node-details.png"),
        fullPage: true,
      });
      await page.goto(
        `/kubernetes/${mapping.cluster}?tab=resources&kind=configmap&namespace_id=${mapping.namespace}&namespace_name=kwr-workloads`,
      );
      await page
        .getByRole("button", {
          name: english ? "Next page" : "下一页",
          exact: true,
        })
        .click();
      await expect(page.locator(".argus-pagination__summary")).toHaveText(
        english ? "Page 2" : "第 2 页",
      );
      await page
        .getByRole("button", {
          name: english ? "Next page" : "下一页",
          exact: true,
        })
        .click();
      await expect(page.locator(".argus-pagination__summary")).toHaveText(
        english ? "Page 3" : "第 3 页",
      );
      const copied = page.url();
      await page.goto("/dashboards");
      await page.goto(copied);
      await expect(page.locator(".argus-pagination__summary")).toHaveText(
        english ? "Page 3" : "第 3 页",
      );
      await page
        .getByRole("button", {
          name: english ? "Previous page" : "上一页",
          exact: true,
        })
        .click();
      await expect(page.locator(".argus-pagination__summary")).toHaveText(
        english ? "Page 2" : "第 2 页",
      );
      await page
        .getByRole("button", {
          name: english ? "Previous page" : "上一页",
          exact: true,
        })
        .click();
      await expect(page.locator(".argus-pagination__summary")).toHaveText(
        english ? "Page 1" : "第 1 页",
      );
      await page.goto(
        `/kubernetes/${mapping.cluster}?tab=resources&kind=cronjob&namespace_id=${mapping.namespace}&namespace_name=kwr-workloads`,
      );
      const cron = page.getByRole("row").filter({
        has: page.getByRole("button", { name: "kwr-cron", exact: true }),
      });
      await selectRowAction(cron, r.actions.run);
      const dialog = page.getByRole("dialog");
      const job = "kwr-closure-" + Date.now().toString(36);
      await dialog.getByRole("textbox", { name: w.jobRun.name }).fill(job);
      await dialog
        .getByRole("button", { name: w.mutation.preview, exact: true })
        .click();
      await dialog
        .getByRole("button", { name: w.mutation.confirm, exact: true })
        .click();
      await expect(dialog).not.toBeVisible();
      await expect(page.locator(".argus-k8s-operation-feedback")).toContainText(
        w.mutation.statuses.succeeded,
        { timeout: 60000 },
      );
      await page
        .locator(".argus-k8s-operation-feedback")
        .getByRole("button", { name: r.openResult, exact: true })
        .click();
      await expect(page).toHaveURL(/resource_operation=/);
      await page.keyboard.press("Escape");
      const nativeArgs = [
        "--kubeconfig",
        root + "/private/drain-kubeconfig.yaml",
        "--context",
        "kind-plan8-drain-kwr-20261113-closure",
        "-n",
        "kwr-workloads",
      ];
      const originalJob = JSON.parse(
        execFileSync(
          "kubectl",
          [...nativeArgs, "get", "job", job, "-o", "json"],
          { encoding: "utf8" },
        ),
      );
      expect(originalJob.metadata.namespace).toBe("kwr-workloads");
      expect(originalJob.metadata.ownerReferences[0].name).toBe("kwr-cron");
      execFileSync(
        "kubectl",
        [
          ...nativeArgs,
          "patch",
          "job",
          job,
          "--type=merge",
          "-p",
          JSON.stringify({ spec: { ttlSecondsAfterFinished: 5 } }),
        ],
        { encoding: "utf8" },
      );
      await expect
        .poll(
          () =>
            execFileSync(
              "kubectl",
              [
                "--kubeconfig",
                root + "/private/drain-kubeconfig.yaml",
                "--context",
                "kind-plan8-drain-kwr-20261113-closure",
                "-n",
                "kwr-workloads",
                "get",
                "job",
                job,
                "--ignore-not-found",
                "-o",
                "name",
              ],
              { encoding: "utf8" },
            ).trim(),
          { timeout: 70000, intervals: [1000, 2000] },
        )
        .toBe("");
      await page
        .getByRole("button", { name: w.mutation.history, exact: true })
        .click();
      let history = page.getByRole("dialog", {
        name: w.mutation.history,
        exact: true,
      });
      await history
        .getByRole("row")
        .filter({ hasText: job })
        .getByRole("button", { name: w.details, exact: true })
        .click();
      await history
        .getByRole("button", { name: r.openResult, exact: true })
        .click();
      drawer = page.getByRole("dialog");
      await expect(
        drawer.getByText(r.resultUnavailable, { exact: true }),
      ).toBeVisible();
      await expect(
        drawer.getByRole("button", { name: r.originalOperation, exact: true }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("reclaimed-job.png"),
        fullPage: true,
      });
      await drawer
        .getByRole("button", { name: r.originalOperation, exact: true })
        .click();
      history = page.getByRole("dialog", {
        name: w.mutation.history,
        exact: true,
      });
      await expect(
        history.getByRole("heading", { name: job, exact: true }),
      ).toBeVisible();
    });
test("namespace reader cannot resolve an ungranted cross-namespace subject", async ({
  page,
}) => {
  test.setTimeout(90000);
  await login(page, false, "light", true);
  await page.goto(
    `/kubernetes/${mapping.cluster}?tab=resources&kind=rolebinding&namespace_id=${mapping.namespace}&namespace_name=kwr-workloads`,
  );
  await page
    .getByRole("button", { name: "kwr-cross", exact: true })
    .first()
    .click();
  const drawer = page.getByRole("dialog");
  let pending = page.waitForResponse(
    (response) =>
      response.url().includes("/inventory/related?") &&
      response.status() === 200,
  );
  await drawer.getByRole("tab", { name: "关联资源", exact: true }).click();
  let data = await (await pending).json();
  for (let round = 0; round < 12; round++) {
    if (data.items.length)
      await expect(
        drawer.getByRole("button", { name: data.items[0].name, exact: true }),
      ).toBeVisible();
    else
      await expect(
        drawer.getByText(
          data.next_cursor
            ? "此页暂无匹配项，可继续读取后续资源"
            : "没有匹配的关联资源或事件",
          { exact: true },
        ),
      ).toBeVisible();
    expect(
      data.items.some(
        (item: { name: string; namespace_id?: string }) =>
          item.name === "cross-account" ||
          item.namespace_id === mapping.second_namespace,
      ),
    ).toBe(false);
    await expect(
      drawer.getByRole("button", { name: "cross-account", exact: true }),
    ).toHaveCount(0);
    if (!data.next_cursor) return;
    pending = page.waitForResponse(
      (response) =>
        response.url().includes("/inventory/related?") &&
        response.status() === 200,
    );
    await drawer.getByRole("button", { name: "下一页", exact: true }).click();
    data = await (await pending).json();
  }
  throw new Error("authorized relation scan did not finish");
});
