import { test, expect, type Page } from "@playwright/test";
import { createHash, randomUUID } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type {
  ConversationInteraction,
  ConversationInteractionState,
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
async function answerQuestions(
  page: Page,
  request: ConversationInteraction,
  directory: string,
) {
  const panel = page.locator(".argus-composer-panel:visible");
  const questions = request.content.questions ?? [];
  for (let index = 0; index < questions.length; index++) {
    await expect(panel).toContainText(questions[index]!.question);
    if (questions[index]!.options.length)
      await panel.getByRole("radio").first().check();
    await panel
      .getByRole("textbox", { name: "补充回答" })
      .fill(
        `同意仅修复 ${directory}/status.conf，将 status=broken 恢复为 status=healthy；保留备份并验证 HTTP 200。不要修改其他目录或主机。`,
      );
    if (index < questions.length - 1)
      await panel
        .getByRole("button", { name: "下一题", exact: true })
        .last()
        .click();
  }
  const accepted = page.waitForResponse(
    (response) =>
      response.url().includes(`/interactions/${request.id}/respond`) &&
      response.request().method() === "POST",
  );
  await panel.getByRole("button", { name: "提交回答", exact: true }).click();
  expect((await accepted).status()).toBe(200);
}
async function approveOrdinaryActions(page: Page): Promise<number> {
  let count = 0;
  for (const card of await page.locator(".argus-command-approval").all()) {
    const confirm = card.getByRole("button", { name: "确认执行", exact: true });
    if (await confirm.isVisible().catch(() => false)) {
      await expect(card).toContainText("AIOps Linux");
      const accepted = page.waitForResponse(
        (response) =>
          response.url().endsWith("/confirm") &&
          response.request().method() === "POST",
      );
      await confirm.click();
      expect((await accepted).status()).toBe(200);
      count++;
    }
  }
  return count;
}
async function waitForPlan(
  page: Page,
  conversation: string,
  directory: string,
  prior?: string,
) {
  const deadline = Date.now() + 600000;
  let questions = 0,
    approvals = 0;
  while (Date.now() < deadline) {
    const view = await state(page, conversation),
      pending = view.items.find((item) => item.status === "pending");
    if (pending?.kind === "user_input") {
      await answerQuestions(page, pending, directory);
      questions++;
    } else if (pending?.kind === "plan" && pending.id !== prior)
      return { plan: pending, questions, approvals };
    else approvals += await approveOrdinaryActions(page);
    if (!view.active_run_id && !pending)
      throw new Error("Agent stopped before producing a guidance plan");
    await page.waitForTimeout(1000);
  }
  throw new Error(
    "Real model did not produce a plan within the acceptance deadline",
  );
}
async function physicalStatus(port: number): Promise<string> {
  const { stdout } = await execute("docker", [
    "exec",
    fixture!.container,
    "curl",
    "-s",
    "-o",
    "/dev/null",
    "-w",
    "%{http_code}",
    `http://127.0.0.1:${port}/`,
  ]);
  return stdout.trim();
}
test.describe("existing-account real guidance-plan implementation", () => {
  test.skip(
    !enabled,
    "Requires the owned Linux fixture and actual account/model",
  );
  test.describe.configure({ mode: "serial", timeout: 1200000 });
  for (const access of ["request_approval", "full_access"] as const) {
    test(`${access}: ask, revise, confirm, ReAct and verify`, async ({
      page,
    }, info) => {
      const owned = fixture!.cases[access]!;
      const { stdout } = await execute("docker", [
        "inspect",
        fixture!.container,
        "--format",
        "{{.Id}}",
      ]);
      expect(stdout.trim()).toBe(fixture!.container_id);
      expect(await physicalStatus(owned.port)).toBe("503");
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
      const modelResponse = await page.request.get(
        "/api/v1/enterprise/ai-models",
      );
      expect(modelResponse.status()).toBe(200);
      const actualModel = (await modelResponse.json()).items.find(
        (model: { id: string }) => model.id === fixture!.model_id,
      );
      expect(actualModel.model_id).toBe("glm-5.3-flash");
      expect(actualModel.base_url).toContain("api.apimart.ai");
      const created = await page.request.post("/api/v1/conversations", {
        headers: {
          "X-CSRF-Token": session.csrf_token,
          "Idempotency-Key": randomUUID(),
          Origin: new URL(page.url()).origin,
        },
        data: {
          title: `Markdown 计划验收 · ${access === "full_access" ? "完全访问" : "请求审批"}`,
          selected_model_id: fixture!.model_id,
        },
      });
      expect(created.status()).toBe(201);
      const conversation = (await created.json()).id as string;
      await page.goto(`/?c=${conversation}`);
      const composer = page.locator(".argus-chat-composer");
      await composer.getByRole("button", { name: "添加", exact: true }).click();
      await page.getByRole("menuitem", { name: /^计划模式/ }).click();
      await expect(
        composer.getByRole("button", { name: "退出计划模式" }),
      ).toBeVisible();
      if (access === "full_access") {
        await composer
          .getByRole("button", { name: "请求审批", exact: true })
          .click();
        await page.getByRole("menuitem", { name: /^完全访问/ }).click();
        await expect(
          composer.getByRole("button", { name: "完全访问", exact: true }),
        ).toBeVisible();
      }
      const prompt = `这是一次真实交互验收。目标是平台已接入的 Docker Linux 主机（host:${fixture!.host_id}）。只允许修复 ${owned.directory}/status.conf，服务地址为目标主机内部 http://127.0.0.1:${owned.port}/，每次请求都读取状态文件；当前应为 HTTP 503。先必须调用 request_user_input，让我选择“修复并验证（推荐）”或“只诊断”，等待真实答复后再继续。然后利用平台主机信息和必要的 connect 只读取证，在 Workspace 分析结果，调用 workflow 分类的 plan.propose 提出完整 Markdown 修复文档。使用 markdown 和 command_bindings；每个待执行的 Bash 多行代码块使用 bash argus:步骤标识，并将该标识与目标主机、bash 解释器一一绑定。把备份、修改和验证分成至少两个独立代码块，必须包含实际命令，不能仅写参考说明，等待我确认。计划确认前不要修复。确认后通过普通 connect 工具逐个引用已批准的 approved_step_id（只传这个 ID，不要重写脚本），观察实际结果继续 ReAct，把 status=broken 改为 status=healthy，保留原配置副本，验证 HTTP 200 和文件内容。最后必须在 Workspace 写入 repair-report.md，报告包含目标目录、修复前 HTTP 503、修复后 HTTP 200 和实际验证证据，并用 workflow.publish_file 发布为 repair-report.md 可下载报告。实际操作遵守本聊天权限，不要伪造确认或结果，不要修改其他资源。`;
      await composer
        .getByRole("textbox", { name: "发送", exact: true })
        .fill(prompt);
      const sent = page.waitForResponse(
        (response) =>
          response.url().endsWith(`/conversations/${conversation}/messages`) &&
          response.request().method() === "POST",
      );
      await composer.getByRole("button", { name: "发送", exact: true }).click();
      expect((await sent).status()).toBe(202);
      const first = await waitForPlan(page, conversation, owned.directory);
      expect(first.questions).toBeGreaterThan(0);
      await expect(page.locator(".argus-guidance-plan")).toHaveCount(1);
      expect(await physicalStatus(owned.port)).toBe("503");
      await composer
        .getByRole("textbox", { name: "修改意见" })
        .fill(
          "请在计划中明确增加：保留原状态配置副本，并展示修复前后的 HTTP 状态码与配置内容对比；完成后用实际结果验证，不能仅凭命令已提交认定成功。",
        );
      await composer
        .getByRole("button", { name: "提交修改意见", exact: true })
        .click();
      const revised = await waitForPlan(
        page,
        conversation,
        owned.directory,
        first.plan.id,
      );
      expect(revised.plan.revision).toBe(first.plan.revision + 1);
      await expect(page.locator(".argus-guidance-plan")).toHaveCount(1);
      await expect(page.locator(".argus-guidance-plan pre code")).toHaveCount(
        revised.plan.content.commands!.length,
      );
      expect(await physicalStatus(owned.port)).toBe("503");
      await page.reload();
      await expect(
        composer.getByRole("button", { name: "执行此计划", exact: true }),
      ).toBeVisible();
      await composer
        .getByRole("button", { name: "执行此计划", exact: true })
        .click();
      await expect(
        composer.getByRole("button", { name: "退出计划模式" }),
      ).toHaveCount(0);
      const deadline = Date.now() + 600000;
      let implementation: ConversationInteraction | undefined,
        approvals = 0;
      while (Date.now() < deadline) {
        const view = await state(page, conversation);
        implementation = view.items.find((item) => item.id === revised.plan.id);
        const input = view.items.find(
          (item) => item.kind === "user_input" && item.status === "pending",
        );
        if (input) await answerQuestions(page, input, owned.directory);
        approvals += await approveOrdinaryActions(page);
        if (implementation?.implementation_status === "succeeded") break;
        if (
          ["failed", "cancelled", "timed_out"].includes(
            implementation?.implementation_status ?? "",
          )
        )
          throw new Error(
            `Implementation ended as ${implementation?.implementation_status}`,
          );
        await page.waitForTimeout(1000);
      }
      expect(implementation?.implementation_status).toBe("succeeded");
      expect(approvals).toBe(0);
      expect(revised.plan.content.commands?.length).toBeGreaterThanOrEqual(2);
      const executionResponse = await page.request.get(
        `/api/v1/conversations/${conversation}/remote-executions`,
      );
      expect(executionResponse.status()).toBe(200);
      const executed = (await executionResponse.json()).items;
      const implementationCommands = executed.filter(
        (x: { spec: { run_id: string } }) =>
          x.spec.run_id === implementation!.implementation_run_id,
      );
      expect(implementationCommands).toHaveLength(
        revised.plan.content.commands!.length,
      );
      expect(
        implementationCommands.every(
          (x: { status: string }) => x.status === "succeeded",
        ),
      ).toBe(true);
      expect(await physicalStatus(owned.port)).toBe("200");
      const config = await execute("docker", [
        "exec",
        fixture!.container,
        "cat",
        `${owned.directory}/status.conf`,
      ]);
      expect(config.stdout).toContain("status=healthy");
      await page.reload();
      await expect(page.locator(".argus-guidance-plan")).toHaveCount(1);
      await expect(
        page
          .getByTestId("chat-message-user")
          .filter({ hasText: "是的，执行此计划" }),
      ).toBeVisible();
      await expect(
        page.locator(".argus-question-summary").first(),
      ).toBeVisible();
      await expect(page.locator(".argus-command-approval")).toHaveCount(0);
      await expect(
        page.getByTestId("tool-trace").filter({ hasText: "远程命令" }),
      ).toHaveCount(0);
      await expect(page.locator(".argus-command-execution")).toHaveCount(
        executed.length,
      );
      await expect(
        page.locator(".argus-guidance-plan .argus-code__head"),
      ).toHaveCount(0);
      await expect(
        page
          .locator(".argus-guidance-plan")
          .getByRole("button", { name: /停止|Stop/ }),
      ).toHaveCount(0);
      const evidenceBeforePlan = await page
        .locator(".argus-command-execution")
        .first()
        .evaluate(
          (element) =>
            !!(
              element.compareDocumentPosition(
                document.querySelector(".argus-guidance-plan")!,
              ) & Node.DOCUMENT_POSITION_FOLLOWING
            ),
        );
      expect(evidenceBeforePlan).toBe(true);
      const downloadPlan = page.waitForEvent("download");
      await page
        .locator(".argus-guidance-plan")
        .getByRole("button", { name: "下载计划文件" })
        .click();
      const savedPlan = await downloadPlan;
      expect(await readFile((await savedPlan.path())!, "utf8")).toBe(
        revised.plan.content.document_markdown,
      );
      const published = page.locator(".argus-file-card").filter({
        has: page.getByRole("button", {
          name: "repair-report.md",
          exact: true,
        }),
      });
      await expect(published).toHaveCount(1);
      const downloadLink = published.getByRole("link", {
        name: "下载 repair-report.md",
      });
      const deliveryUrl = await downloadLink.getAttribute("href");
      expect(deliveryUrl).toContain("/deliveries/");
      const downloadEvent = page.waitForEvent("download");
      await downloadLink.click();
      const downloaded = await downloadEvent;
      expect(downloaded.suggestedFilename()).toBe("repair-report.md");
      const reportBytes = await readFile((await downloaded.path())!);
      const reportText = reportBytes.toString("utf8");
      expect(reportText).toContain(owned.directory);
      expect(reportText).toContain("503");
      expect(reportText).toContain("200");
      // A later ordinary command must not inherit the previous Run's grants.
      await composer
        .getByRole("textbox", { name: "发送", exact: true })
        .fill(
          `新增一次独立检查：在同一主机 host:${fixture!.host_id} 用一次普通 connect 执行 printf 'post-plan-check\\n'; cat '${owned.directory}/status.conf'，然后简短报告结果。不要引用前一实施 Run 的 approved_step_id，不要创建新计划。`,
        );
      const nextAccepted = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/conversations/${conversation}/messages`) &&
          r.request().method() === "POST",
      );
      await composer.getByRole("button", { name: "发送", exact: true }).click();
      const nextResponse = await nextAccepted;
      expect(nextResponse.status()).toBe(202);
      const nextRun = (await nextResponse.json()).run.run_id;
      let additionalApprovals = 0;
      await expect
        .poll(
          async () => {
            additionalApprovals += await approveOrdinaryActions(page);
            const response = await page.request.get(`/api/v1/runs/${nextRun}`);
            expect(response.status()).toBe(200);
            return (await response.json()).status;
          },
          { timeout: 600000, intervals: [1000] },
        )
        .toBe("succeeded");
      expect(additionalApprovals).toBe(access === "request_approval" ? 1 : 0);
      await expect(page.locator(".argus-command-approval")).toHaveCount(0);
      await expect(page.locator(".argus-command-execution")).toHaveCount(
        executed.length + 1,
      );
      await page
        .locator(".argus-command-execution")
        .last()
        .getByRole("button", { name: "展开命令执行详情" })
        .click();
      await expect(
        page.locator(".argus-command-execution").last(),
      ).toContainText("post-plan-check");
      await expect(
        page.locator(".argus-command-execution").last(),
      ).toContainText("status=healthy");
      const report = {
        status: "passed",
        access_mode: access,
        conversation,
        url: new URL(`/?c=${conversation}`, page.url()).href,
        model: fixture!.model_id,
        plan_id: revised.plan.id,
        revision: revised.plan.revision,
        implementation_run_id: implementation!.implementation_run_id,
        question_batches: first.questions + revised.questions,
        implementation_ordinary_confirmations: approvals,
        additional_ordinary_confirmations: additionalApprovals,
        additional_run_id: nextRun,
        physical_http_status: "200",
        directory: owned.directory,
        delivery_url: deliveryUrl,
        downloaded_bytes: reportBytes.length,
        downloaded_sha256: createHash("sha256")
          .update(reportBytes)
          .digest("hex"),
      };
      await writeFile(
        info.outputPath("real-interaction.json"),
        JSON.stringify(report, null, 2),
      );
      await page.screenshot({
        path: info.outputPath("real-interaction.png"),
        fullPage: true,
        animations: "disabled",
      });
    });
  }
});
