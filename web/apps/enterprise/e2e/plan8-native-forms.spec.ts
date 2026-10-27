import { expect, test } from "@playwright/test";

for (const item of [
  {
    type: "Oracle",
    endpoint: "oracle.example:1521",
    extra: "Oracle service name",
    value: "FREEPDB1",
    username: true,
    password: true,
  },
  {
    type: "Kafka",
    endpoint: "kafka-a.example:9092",
    extra: "Additional member addresses",
    value: "kafka-b.example:9092",
    username: true,
    password: true,
  },
  {
    type: "DNS",
    endpoint: "dns.example:53",
    extra: "Names to resolve",
    value: "example.org, example.net",
    username: false,
    password: false,
  },
  {
    type: "Apache HTTP Server",
    endpoint: "https://apache.example/server-status?auto",
    username: false,
    password: false,
  },
]) {
  test(`Plan8 native ${item.type} form uses component fields`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.addInitScript(() =>
      localStorage.setItem("argus.locale", "en-US"),
    );
    await page.goto("/login");
    await page.locator('input[autocomplete="username"]').fill("root");
    await page.locator('input[autocomplete="current-password"]').fill("123456");
    await page.locator('form button[type="submit"]').click();
    await expect(page).not.toHaveURL(/\/login/);
    await page.goto("/monitoring");
    await page
      .getByRole("button", { name: "New monitor", exact: true })
      .click();
    const catalog = page.getByRole("dialog", {
      name: "Choose a monitoring type",
    });
    await catalog.getByRole("textbox").fill(item.type);
    await catalog
      .getByRole("row")
      .filter({ has: page.getByText(item.type, { exact: true }) })
      .getByRole("button")
      .click();
    const form = page.getByRole("dialog", {
      name: `New ${item.type} monitor`,
      exact: true,
    });
    await form
      .getByRole("textbox", { name: /Monitor name/ })
      .fill(`${item.type} acceptance`);
    await form
      .getByRole("textbox", { name: /Service endpoint/ })
      .fill(item.endpoint);
    if (item.extra)
      await form
        .getByRole("textbox", { name: new RegExp(item.extra) })
        .fill(item.value!);
    if (item.username)
      await form.getByRole("textbox", { name: /Username/ }).fill("monitor");
    if (item.password)
      await form
        .locator('input[type="password"]')
        .fill("literal-${env:HOME}-test-only");
    await expect(
      form.getByRole("textbox", { name: "Environment", exact: true }),
    ).toHaveCount(0);
    await form
      .getByRole("button", { name: "Choose a Collector", exact: true })
      .click();
    await page
      .getByRole("dialog", { name: "Choose a Collector", exact: true })
      .getByRole("button", { name: /^Select / })
      .first()
      .click();
    await form
      .getByRole("button", { name: "Preview configuration", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Confirm changes", exact: true })
      .click();
    await expect(
      page
        .getByRole("row")
        .filter({
          has: page.getByText(`${item.type} acceptance`, { exact: true }),
        }),
    ).toBeVisible();
    expect(
      await page.evaluate(() => JSON.stringify(localStorage)),
    ).not.toContain("literal-${env:HOME}-test-only");
  });
}
