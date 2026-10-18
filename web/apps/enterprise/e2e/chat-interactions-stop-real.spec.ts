import { test, expect, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { writeFile } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type {
  ConversationInteractionState,
  RemoteExecution,
} from "@argus/api-client";
import { plan7Login } from "./helpers/plan7-login";

const execute = promisify(execFile);
const enabled = process.env.ARGUS_CHAT_INTERACTIONS_REAL === "1";
const fixture = enabled
  ? (JSON.parse(process.env.ARGUS_CHAT_INTERACTIONS_FIXTURE!) as {
      container: string;
      container_id: string;
      host_id: string;
      model_id: string;
      cases: Record<string, { directory: string; port: number }>;
    })
  : undefined;
async function state(
  page: Page,
  conversation: string,
): Promise<ConversationInteractionState> {
  const response = await page.request.get(
    `/api/v1/conversations/${conversation}/interactions`,
  );
  expect(response.status()).toBe(200);
  return response.json();
}
async function markers(directory: string) {
  const { stdout } = await execute("docker", [
    "exec",
    fixture!.container,
    "sh",
    "-c",
    `for f in started finished follow-up; do if test -f '${directory}/'$f; then printf '%s\\n' "$f"; fi; done`,
  ]);
  return stdout.trim().split(/\s+/).filter(Boolean);
}

test.describe("real guidance implementation stop", () => {
  test.skip(!enabled, "Requires the owned Linux fixture and actual model");
  test.describe.configure({ mode: "serial", timeout: 1200000 });
  for (const access of ["request_approval", "full_access"] as const) {
    test(`${access}: reload recovers implementation Run, stop cancels command and later work`, async ({
      page,
    }, info) => {
      const { stdout } = await execute("docker", [
        "inspect",
        fixture!.container,
        "--format",
        "{{.Id}}",
      ]);
      expect(stdout.trim()).toBe(fixture!.container_id);
      const directory = `${fixture!.cases[access]!.directory}-stop`;
      expect(directory).toMatch(/^\/tmp\/argus-chat-interactions-[a-z0-9_-]+$/);
      // mkdir without -p refuses to overwrite any prior acceptance fixture.
      await execute("docker", ["exec", fixture!.container, "mkdir", directory]);
      await page.addInitScript(() => {
        localStorage.setItem("argus.locale", "zh-CN");
        localStorage.setItem("argus.theme", "light");
      });
      await plan7Login(
        page,
        "/login",
        process.env.ARGUS_PLAN7_USERNAME!,
        process.env.ARGUS_PLAN7_PASSWORD!,
      );
      const sessionResponse = await page.request.get(
        "/api/v1/enterprise/auth/session",
      );
      expect(sessionResponse.status()).toBe(200);
      const session = await sessionResponse.json();
      expect(session.user.username).toBe(process.env.ARGUS_PLAN7_USERNAME);
      const created = await page.request.post("/api/v1/conversations", {
        headers: {
          "X-CSRF-Token": session.csrf_token,
          "Idempotency-Key": randomUUID(),
          Origin: new URL(page.url()).origin,
        },
        data: {
          title: `停止验收 · ${access}`,
          selected_model_id: fixture!.model_id,
        },
      });
      expect(created.status()).toBe(201);
      const conversation = (await created.json()).id as string;
      await page.goto(`/?c=${conversation}`);
      const composer = page.locator(".argus-chat-composer");
      await composer.getByRole("button", { name: "添加", exact: true }).click();
      await page.getByRole("menuitem", { name: /^计划模式/ }).click();
      if (access === "full_access") {
        await composer
          .getByRole("button", { name: "请求审批", exact: true })
          .click();
        await page.getByRole("menuitem", { name: /^完全访问/ }).click();
      }
      const prompt = `在已接入的 Linux 主机 host:${fixture!.host_id} 上进行一次受控停止验收，只能使用已经创建的目录 ${directory}。请直接通过 workflow 的 plan.propose 提出以下 Markdown 计划并等待确认。通过 markdown 字段提供正文，两个命令分别使用 bash argus:slow 和 bash argus:follow 代码围栏，并用 command_bindings 绑定同一主机和 bash 解释器，无需询问、无需先执行命令。第一步：用一次 connect 在 Bash 执行 printf started > ${directory}/started; sleep 90; printf finished > ${directory}/finished，命令超时设为至少 120 秒。第二步必须等待第一步结束后再用另一次 connect 创建 ${directory}/follow-up。计划确认后逐个用 connect 只传 approved_step_id 执行原命令，不再重写脚本；我会主动停止来检查中断，停止后不要继续执行任何操作。不得使用后台进程、重试原命令或操作其他目录。`;
      await composer
        .getByRole("textbox", { name: "发送", exact: true })
        .fill(prompt);
      await composer.getByRole("button", { name: "发送", exact: true }).click();
      await expect
        .poll(
          async () =>
            (await state(page, conversation)).items.filter(
              (x) => x.kind === "plan" && x.status === "pending",
            ).length,
          { timeout: 600000 },
        )
        .toBe(1);
      expect(await markers(directory)).toEqual([]);
      await composer
        .getByRole("button", { name: "执行此计划", exact: true })
        .click();
      let approved = 0;
      await expect
        .poll(
          async () => {
            if (access === "request_approval") {
              for (const card of await page
                .locator(".argus-command-approval")
                .all()) {
                const button = card.getByRole("button", {
                  name: "确认执行",
                  exact: true,
                });
                if (await button.isVisible().catch(() => false)) {
                  await expect(card).toContainText(directory);
                  await expect(card).toContainText("AIOps Linux");
                  await button.click();
                  approved++;
                }
              }
            }
            return (await markers(directory)).includes("started");
          },
          { timeout: 600000, intervals: [1000] },
        )
        .toBe(true);
      const before = await state(page, conversation);
      const plan = before.items.find((x) => x.status === "confirmed")!;
      expect(before.active_run_id).toBe(plan.implementation_run_id);
      expect(plan.run_id).not.toBe(plan.implementation_run_id);
      await page.reload();
      const stop = composer.getByRole("button", {
        name: "停止任务",
        exact: true,
      });
      await expect(stop).toBeVisible();
      expect((await state(page, conversation)).active_run_id).toBe(
        plan.implementation_run_id,
      );
      expect(await markers(directory)).toEqual(["started"]);
      const cancelled = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/runs/${plan.implementation_run_id}/cancel`) &&
          r.request().method() === "POST",
      );
      await stop.click();
      expect((await cancelled).status()).toBe(200);
      await expect
        .poll(
          async () =>
            (await state(page, conversation)).items.find(
              (x) => x.id === plan.id,
            )?.implementation_status,
        )
        .toBe("cancelled");
      await expect(stop).toHaveCount(0);
      await expect(
        page.getByTestId("tool-trace").filter({ hasText: "远程命令" }),
      ).toHaveCount(0);
      let requests: RemoteExecution[] = [];
      await expect
        .poll(
          async () => {
            const response = await page.request.get(
              `/api/v1/conversations/${conversation}/remote-executions`,
            );
            expect(response.status()).toBe(200);
            requests = (await response.json()).items;
            return (
              requests.length > 0 &&
              requests.every((r) => r.status === "cancelled")
            );
          },
          { timeout: 90000, intervals: [1000] },
        )
        .toBe(true);
      const requestIds = requests.map((r) => r.id);
      await page.reload();
      await expect(page.locator(".argus-guidance-plan")).toContainText(
        "已确认",
      );
      await expect(stop).toHaveCount(0);
      // Allow cancellation watchers and any queued worker tasks to settle.
      await page.waitForTimeout(5000);
      expect(await markers(directory)).toEqual(["started"]);
      const after = await page.request.get(
        `/api/v1/conversations/${conversation}/remote-executions`,
      );
      expect(
        (await after.json()).items.map((r: RemoteExecution) => r.id),
      ).toEqual(requestIds);
      expect((await state(page, conversation)).active_run_id).toBeUndefined();
      expect(approved).toBe(0);
      await writeFile(
        info.outputPath("real-stop.json"),
        JSON.stringify(
          {
            status: "passed",
            conversation,
            access_mode: access,
            plan_id: plan.id,
            planning_run_id: plan.run_id,
            implementation_run_id: plan.implementation_run_id,
            request_ids: requestIds,
            requests,
            markers: await markers(directory),
            directory,
          },
          null,
          2,
        ),
      );
      await page.screenshot({
        path: info.outputPath("real-stop.png"),
        fullPage: true,
      });
    });
  }
});
