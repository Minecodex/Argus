import { expect, test } from "@playwright/test";
test("audit restored paginated list and existing Role details", async ({
  page,
}, info) => {
  test.setTimeout(90000);
  page.setDefaultTimeout(15000);
  await page.setViewportSize({ width: 1440, height: 1080 });
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/kubernetes/k8s-prod-east?tab=resources&kind=pod");
  await expect(
    page.getByRole("button", { name: "pod-example", exact: true }).first(),
  ).toBeVisible();
  await page.evaluate(() => {
    const key = "argus-mock:db-v14",
      db = JSON.parse(localStorage.getItem(key)!);
    const state = db.kubernetesInventory["k8s-prod-east"],
      source = state.objects.find((item: any) => item.definition.key === "pod");
    for (let index = 0; index < 40; index++) {
      const item = structuredClone(source);
      item.uid = "review-pod-uid-" + index;
      item.name = "review-pod-" + String(index).padStart(2, "0");
      item.data.metadata.uid = item.uid;
      item.data.metadata.name = item.name;
      state.objects.push(item);
    }
    const role = state.objects.find(
      (item: any) => item.definition.key === "role",
    );
    role.data.rules = [
      {
        apiGroups: ["apps"],
        resources: ["deployments"],
        verbs: ["get", "list"],
      },
    ];
    localStorage.setItem(key, JSON.stringify(db));
  });
  await page.reload();
  await expect(
    page.getByRole("button", { name: "下一页", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await expect(page.locator(".argus-pagination__summary")).toHaveText(
    "第 2 页",
  );
  const url = page.url();
  await expect(
    page.getByRole("button", { name: "上一页", exact: true }),
  ).toBeEnabled();
  const first = await page
    .locator(".argus-k8s-resource-name-link")
    .first()
    .textContent();
  await page.getByRole("link", { name: "仪表盘", exact: true }).click();
  await expect(page).toHaveURL(/\/dashboards/);
  await page.goBack();
  await expect(page).toHaveURL(url);
  await expect(
    page.locator(".argus-k8s-resource-name-link").first(),
  ).toHaveText(first!);
  await page.reload();
  await expect(
    page.locator(".argus-k8s-resource-name-link").first(),
  ).toHaveText(first!);
  const observation = {
    url_preserved: page.url() === url,
    rows_preserved: true,
    page_label: await page.locator(".argus-pagination__summary").textContent(),
    previous_disabled: await page
      .getByRole("button", { name: "上一页", exact: true })
      .isDisabled(),
  };
  console.log("PAGINATION_OBSERVATION", JSON.stringify(observation));
  await info.attach("pagination.json", {
    body: JSON.stringify(observation),
    contentType: "application/json",
  });
  await page.screenshot({
    path: info.outputPath("restored-list.png"),
    fullPage: true,
  });
  await page.goto("/kubernetes/k8s-prod-east?tab=resources&kind=role");
  await page
    .getByRole("button", { name: "role-example", exact: true })
    .first()
    .click();
  const drawer = page.getByRole("dialog");
  await drawer.getByRole("tab", { name: "配置", exact: true }).click();
  const detail = {
    configuration_text: await drawer.textContent(),
    tables: await drawer.getByRole("table").count(),
  };
  console.log("ROLE_CONFIGURATION", JSON.stringify(detail));
  await info.attach("role-configuration.json", {
    body: JSON.stringify(detail),
    contentType: "application/json",
  });
  await page.screenshot({
    path: info.outputPath("role-configuration.png"),
    fullPage: true,
  });
  await page.evaluate(() => {
    const key = "argus-mock:db-v14",
      db = JSON.parse(localStorage.getItem(key)!),
      state = db.kubernetesInventory["k8s-prod-east"],
      ns = state.namespaces[0],
      now = new Date().toISOString();
    state.operations = [
      {
        id: "review-operation",
        cluster_id: "k8s-prod-east",
        namespace_id: ns.id,
        namespace_uid: ns.uid,
        status: "succeeded",
        operation: "create",
        type: "job",
        name: "recycled-job",
        uid: "gone-job-uid",
        error_code: "",
        version: 1,
        result: {
          uid: "gone-job-uid",
          after: {
            kind: "Job",
            metadata: { name: "recycled-job", namespace: ns.name },
          },
        },
        created_at: now,
        updated_at: now,
      },
    ];
    localStorage.setItem(key, JSON.stringify(db));
  });
  await page.goto("/kubernetes/k8s-prod-east?tab=resources&kind=job");
  await page.getByRole("button", { name: "操作记录", exact: true }).click();
  const history = page.getByRole("dialog", { name: "操作记录", exact: true });
  await history.getByRole("button", { name: "查看对象", exact: true }).click();
  await history
    .getByRole("button", { name: "查看结果资源", exact: true })
    .click();
  const gone = page.getByRole("dialog");
  await expect(
    gone.getByText("原操作结果对象已不可用", { exact: true }),
  ).toBeVisible();
  const lifecycle = {
    text: await gone.textContent(),
    operation_detail_links: await gone
      .getByRole("button", { name: "查看原操作记录", exact: true })
      .count(),
  };
  console.log("RECLAIMED_JOB", JSON.stringify(lifecycle));
  await info.attach("reclaimed-job.json", {
    body: JSON.stringify(lifecycle),
    contentType: "application/json",
  });
  await page.screenshot({
    path: info.outputPath("reclaimed-job.png"),
    fullPage: true,
  });
  expect(detail.tables).toBe(1);
  expect(lifecycle.operation_detail_links).toBe(1);
  expect(lifecycle.text).toContain("原对象可能已被控制器回收");
  expect(
    observation.previous_disabled,
    "reloading a page-2 URL must still allow previous page",
  ).toBe(false);
});
