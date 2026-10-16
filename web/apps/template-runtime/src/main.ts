import { startRuntime, trustedHello } from "./runtime";
let connected = false;
window.addEventListener("message", (event: MessageEvent<unknown>) => {
  if (
    connected ||
    !trustedHello(
      event,
      window.parent,
      new URLSearchParams(location.search).get("parent_origin"),
    )
  )
    return;
  connected = true;
  const port = event.ports[0]!;
  void startRuntime(event.data, port).catch(() =>
    port.postMessage({
      version: event.data.version,
      nonce: event.data.nonce,
      sequence: 1,
      request_id: crypto.randomUUID(),
      type: "error",
      payload: { code: "TEMPLATE_LOAD_FAILED" },
    }),
  );
});
