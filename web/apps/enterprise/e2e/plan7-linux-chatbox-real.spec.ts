import { test, expect } from "@playwright/test";
import type { RemoteExecution } from "@argus/api-client";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { writeFile } from "node:fs/promises";
import { plan7Login } from "./helpers/plan7-login";
import { dashboardAPI } from "./helpers/planv2-api";
import { setChoice } from "./helpers/choice";
import { createMfaProof } from "./helpers/mfa-login";

const execute = promisify(execFile);

test("real model diagnoses and repairs Docker Linux through Chatbox approvals", async ({
  page,
}, info) => {
  test.skip(
    process.env.ARGUS_PLAN7_LINUX_CHATBOX !== "1",
    "Requires the owned Linux and selected real model",
  );
  test.setTimeout(40 * 60_000);
  const fixture = JSON.parse(process.env.ARGUS_PLAN7_LINUX_FIXTURE!) as {
    conversation: string;
    resource_id: string;
    directory: string;
    unit: string;
    container: string;
    owner: string;
    model: string;
  };
  const owned = JSON.parse(
    (await execute("docker", ["inspect", fixture.container])).stdout,
  );
  expect(owned).toHaveLength(1);
  expect(owned[0].Id).toBe(fixture.container);
  expect(owned[0].Config.Labels["io.argus.e2e.run"]).toBe(fixture.owner);
  const workers = async () =>
    (
      await execute("docker", [
        "exec",
        fixture.container,
        "cat",
        `${fixture.directory}/workers`,
      ])
    ).stdout.trim();
  expect(await workers()).toBe("0");
  await page.addInitScript(() => {
    localStorage.setItem("argus.locale", "zh-CN");
    localStorage.setItem("argus.theme", "light");
  });
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await plan7Login(
    page,
    "/login",
    process.env.ARGUS_PLAN7_USERNAME!,
    process.env.ARGUS_PLAN7_PASSWORD!,
  );
  await page.goto(`/?c=${fixture.conversation}`);
  await expect(
    page.getByRole("radio", { name: "排查", exact: true }),
  ).toBeChecked();
  const requests = async (): Promise<RemoteExecution[]> =>
    (
      await (
        await dashboardAPI(
          page,
          `/conversations/${fixture.conversation}/remote-executions`,
        )
      ).json()
    ).items;
  const approved: RemoteExecution[] = [];
  const nextMfaProof = createMfaProof("enterprise");
  const preview = async (view: RemoteExecution) => {
    expect(view.status).toBe("awaiting_confirmation");
    expect(view.spec.steps.length).toBeGreaterThan(0);
    for (const step of view.spec.steps) {
      expect(step.target.resource_type).toBe("host");
      expect(step.target.resource_id).toBe(fixture.resource_id);
      expect(step.interpreter).toBe("bash");
    }
    for (const result of view.results) expect(result.status).toBe("pending");
    // Feedback changes the action's label. Bind the card to its immutable
    // title/revision so filling feedback cannot invalidate its own locator.
    const card = page.getByRole("region", {
      name: `${view.spec.title} · 计划版本 ${view.revision}`,
      exact: true,
    });
    await expect(card).toHaveCount(1);
    await expect(card.locator(".argus-command-plan__steps > li")).toHaveCount(
      view.spec.steps.length,
    );
    for (let n = 0; n < view.spec.steps.length; n++) {
      await expect(
        card
          .locator(".argus-command-plan__steps > li")
          .nth(n)
          .locator(".argus-code pre code")
          .first(),
      ).toHaveText(view.spec.steps[n].script);
    }
    return card;
  };
  const approve = async (view: RemoteExecution) => {
    const card = await preview(view);
    const response = page.waitForResponse(
      (r) =>
        r.url().includes(`/pending-actions/${view.action_ref}/confirm`) &&
        r.request().method() === "POST",
    );
    await card.getByRole("button", { name: "确认执行", exact: true }).click();
    const confirmed = await response;
    if (confirmed.status() === 403) {
      expect((await confirmed.json()).code).toBe("STEP_UP_REQUIRED");
      const dialog = page.getByRole("dialog");
      await dialog
        .locator('input[autocomplete="one-time-code"]')
        .fill(await nextMfaProof());
      const retried = page.waitForResponse(
        (r) =>
          r.url().includes(`/pending-actions/${view.action_ref}/confirm`) &&
          r.request().method() === "POST",
      );
      await dialog
        .getByRole("button", { name: "验证并继续", exact: true })
        .click();
      expect((await retried).status()).toBe(200);
    } else {
      expect(confirmed.status()).toBe(200);
    }
    approved.push(view);
  };
  const send = async (content: string) => {
    const composer = page.locator(".argus-chat-composer");
    await composer.getByRole("textbox").fill(content);
    const accepted = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/conversations/${fixture.conversation}/messages`) &&
        r.request().method() === "POST",
    );
    await composer.getByRole("button", { name: "发送", exact: true }).click();
    const response = await accepted;
    expect(response.status()).toBe(202);
    return (await response.json()).run.run_id as string;
  };
  const waitRun = async (run: string, maximum: number) => {
    const seen = new Set(approved.map((view) => view.id));
    const started = approved.length;
    await expect
      .poll(
        async () => {
          for (const view of await requests()) {
            if (
              view.spec.run_id !== run ||
              view.status !== "awaiting_confirmation" ||
              seen.has(view.id)
            )
              continue;
            expect(approved.length - started).toBeLessThan(maximum);
            await approve(view);
            seen.add(view.id);
          }
          const value = await (await dashboardAPI(page, `/runs/${run}`)).json();
          if (["failed", "cancelled", "timed_out"].includes(value.status))
            throw new Error(
              `Actual model Run ended: ${value.status}/${value.error_code}/${value.stop_reason}`,
            );
          return value.status;
        },
        { timeout: 600_000, intervals: [1000] },
      )
      .toBe("succeeded");
  };
  const waitPreview = async (run: string, prior?: string) => {
    let found: RemoteExecution | undefined;
    await expect
      .poll(
        async () => {
          found = (await requests()).find(
            (view) =>
              view.spec.run_id === run &&
              view.status === "awaiting_confirmation" &&
              view.id !== prior,
          );
          if (!found) {
            const value = await (
              await dashboardAPI(page, `/runs/${run}`)
            ).json();
            expect([
              "failed",
              "cancelled",
              "timed_out",
              "succeeded",
            ]).not.toContain(value.status);
          }
          return !!found;
        },
        { timeout: 600_000, intervals: [1000] },
      )
      .toBe(true);
    return found!;
  };

  const diagnosisRun = await send(
    `只排查主机资源 ${fixture.resource_id} 的本轮专用 systemd 服务 ${fixture.unit}，健康检查 ${fixture.directory}/health，配置 ${fixture.directory}/workers，正确 workers 应为 2。取得现场状态、相关日志和配置证据，复制到 ARGUS_OUTPUT_DIR 由平台自动回传，在 Workspace 分析并说明原因。现在禁止修改配置、备份或重启；只操作该测试目录及该服务，每次 connect 都等确认。状态失败本身也是证据，保留退出码并让取证脚本正常返回。不要读取平台身份文件或其他资源。`,
  );
  await waitRun(diagnosisRun, 3);
  expect(approved.length).toBeGreaterThan(0);
  expect(await workers()).toBe("0");
  await expect(page.locator(".argus-file-card").first()).toBeVisible();
  await page.screenshot({
    path: info.outputPath("01-real-model-diagnosis.png"),
    fullPage: true,
  });

  await setChoice(page.getByRole("radio", { name: "计划", exact: true }));
  const repairRun = await send(
    `根据已经确认的证据生成修复计划：将 ${fixture.directory}/workers 改为 2，重启 ${fixture.unit}，再验证进程实际运行和 ${fixture.directory}/health。用多个步骤描述＋完整 Bash 命令组成固定计划，只操作此测试目录和此服务，先等一次确认再串行执行；执行后在 Workspace 检查自动回传证据并汇报。`,
  );
  const original = await waitPreview(repairRun);
  expect(original.spec.kind).toBe("plan");
  expect(original.spec.steps.length).toBeGreaterThan(1);
  const originalCard = await preview(original);
  expect(await workers()).toBe("0");
  await page.screenshot({
    path: info.outputPath("02-original-plan-unapproved.png"),
    fullPage: true,
  });
  await originalCard
    .getByRole("textbox")
    .fill(
      "请先将 workers 备份为同目录 workers.bak，再修改配置，重启并验证 health。其他目标和限制不变，仍然先等待确认。",
    );
  const feedback = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/${original.id}/feedback`) &&
      r.request().method() === "POST",
  );
  await originalCard
    .getByRole("button", { name: "提交修改意见", exact: true })
    .click();
  expect((await feedback).status()).toBe(202);
  const revised = await waitPreview(repairRun, original.id);
  expect(revised.spec.kind).toBe("plan");
  expect(revised.spec.supersedes_id).toBe(original.id);
  expect(revised.revision).toBe(original.revision + 1);
  expect(revised.spec.steps.map((step) => step.script).join("\n")).toContain(
    "workers.bak",
  );
  expect(await workers()).toBe("0");
  await preview(revised);
  await page.screenshot({
    path: info.outputPath("03-revised-plan-unapproved.png"),
    fullPage: true,
  });
  await approve(revised);
  // Reload while the server owns the approved execution. This must neither
  // cancel the plan nor create a second approval or command dispatch.
  await page.reload();
  await waitRun(repairRun, 1);
  const final = await requests();
  expect(final.find((view) => view.id === original.id)?.status).toBe(
    "superseded",
  );
  const completed = final.find((view) => view.id === revised.id)!;
  expect(completed.status).toBe("succeeded");
  expect(completed.results).toHaveLength(completed.spec.steps.length);
  for (const result of completed.results) {
    expect(result.status).toBe("succeeded");
    expect(result.terminated).toBe(true);
    expect(result.outputs_collected).toBe(true);
  }
  expect(approved.filter((view) => view.spec.kind === "plan")).toHaveLength(1);
  expect(await workers()).toBe("2");
  expect(
    (
      await execute("docker", [
        "exec",
        fixture.container,
        "cat",
        `${fixture.directory}/workers.bak`,
      ])
    ).stdout.trim(),
  ).toBe("0");
  expect(
    (
      await execute("docker", [
        "exec",
        fixture.container,
        `${fixture.directory}/health`,
      ])
    ).stdout.trim(),
  ).toBe("healthy");
  await expect(
    page.locator(".argus-command-plan").filter({ hasText: "已被新版本替代" }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("radio", { name: "计划", exact: true }),
  ).toBeChecked();
  await page.screenshot({
    path: info.outputPath("04-repaired-after-reload.png"),
    fullPage: true,
  });
  expect(errors).toEqual([]);
  await writeFile(
    process.env.ARGUS_PLAN7_FLOW_REPORT!,
    JSON.stringify(
      {
        scope:
          "real browser composer, revision and approval; actual Docker Linux effect",
        model: fixture.model,
        conversation: fixture.conversation,
        diagnosis_run: diagnosisRun,
        repair_run: repairRun,
        original,
        revised,
        completed,
        approved,
        diagnosis_preserved_configuration: true,
        configuration_unchanged_until_approval: true,
        reload_did_not_cancel: true,
        actual_health: "healthy",
        page_errors: errors,
      },
      null,
      2,
    ),
  );
});
