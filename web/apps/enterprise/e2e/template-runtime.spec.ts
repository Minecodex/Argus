import { expect, test } from "@playwright/test";
import { templateOrigin } from "./origins";

test("Template runtime isolates business data, blocks network and has no confirmation capability", async ({
  page,
}) => {
  await page.goto("/login");
  await page.evaluate(async (origin) => {
    const iframe = document.createElement("iframe");
    iframe.id = "template-proof";
    iframe.setAttribute("sandbox", "allow-scripts");
    const source = `<output id="value"></output><output id="network"></output><output id="authority"></output><script>
      ArgusTemplate.onData((data)=>{document.getElementById('value').textContent=data.value;});
      document.getElementById('authority').textContent=String(typeof ArgusTemplate.confirm_action);
      fetch('https://example.com').then(()=>document.getElementById('network').textContent='escaped').catch(()=>document.getElementById('network').textContent='blocked');
    </script>`;
    const hash = Array.from(
      new Uint8Array(
        await crypto.subtle.digest("SHA-256", new TextEncoder().encode(source)),
      ),
      (x) => x.toString(16).padStart(2, "0"),
    ).join("");
    const loaded = new Promise<void>((resolve) =>
      iframe.addEventListener("load", () => resolve(), { once: true }),
    );
    iframe.src = `${origin}/?parent_origin=${encodeURIComponent(location.origin)}`;
    document.body.append(iframe);
    await loaded;
    const channel = new MessageChannel();
    channel.port1.start();
    iframe.contentWindow!.postMessage(
      {
        version: "argus-template/v1",
        nonce: crypto.randomUUID(),
        sequence: 1,
        request_id: crypto.randomUUID(),
        type: "hello",
        payload: {
          template_source: source,
          template_hash: hash,
          detail_data: { value: "<script>data must remain text</script>" },
          context: { locale: "zh-CN", color_scheme: "dark", tokens: {} },
        },
      },
      "*",
      [channel.port2],
    );
  }, templateOrigin);
  const frame = page.frameLocator("#template-proof");
  await expect(frame.locator("#value")).toHaveText(
    "<script>data must remain text</script>",
  );
  await expect(frame.locator("#network")).toHaveText("blocked");
  await expect(frame.locator("#authority")).toHaveText("undefined");
});

test("Template runtime rejects an invalid hash and the wrong parent origin", async ({
  page,
}) => {
  await page.goto("/login");
  const result = await page.evaluate(async (origin) => {
    const attempt = async (expectedOrigin: string) => {
      const iframe = document.createElement("iframe");
      iframe.setAttribute("sandbox", "allow-scripts");
      const loaded = new Promise<void>((resolve) =>
        iframe.addEventListener("load", () => resolve(), { once: true }),
      );
      iframe.src = `${origin}/?parent_origin=${encodeURIComponent(expectedOrigin)}`;
      document.body.append(iframe);
      await loaded;
      const channel = new MessageChannel();
      const result = new Promise<string>((resolve) => {
        const timeout = setTimeout(() => resolve("ignored"), 1000);
        channel.port1.onmessage = (event) => {
          clearTimeout(timeout);
          resolve(String(event.data.type));
        };
      });
      channel.port1.start();
      iframe.contentWindow!.postMessage(
        {
          version: "argus-template/v1",
          nonce: crypto.randomUUID(),
          sequence: 1,
          request_id: crypto.randomUUID(),
          type: "hello",
          payload: {
            template_source: "<p>forged</p>",
            template_hash: "0".repeat(64),
            detail_data: {},
            context: { locale: "en-US", color_scheme: "light", tokens: {} },
          },
        },
        "*",
        [channel.port2],
      );
      const value = await result;
      channel.port1.close();
      iframe.remove();
      return value;
    };
    return {
      hash: await attempt(location.origin),
      origin: await attempt("https://other.invalid"),
    };
  }, templateOrigin);
  expect(result).toEqual({ hash: "error", origin: "ignored" });
});
