import { WebSocketTransport } from "./websocket";
import type { KubernetesStreamTicket } from "../kubernetes-workbench";

const protocol = "argus.kubernetes_stream/v1";
export class KubernetesTerminalConnection {
  private transport: WebSocketTransport;
  private nonce = "";
  private sent = 1;
  private received = 0;
  private decoder = new TextDecoder();
  private ended = false;
  constructor(
    private callbacks: {
      output: (data: string) => void;
      state: (
        state: "connecting" | "connected" | "disconnected",
        reason?: string,
      ) => void;
    },
  ) {
    this.transport = new WebSocketTransport({
      max_message_bytes: 64 * 1024,
      on_message: (data) => this.receive(data),
      on_close: (event) => this.end(event.reason),
      on_error: () => this.end("KUBERNETES_STREAM_BACKEND_UNAVAILABLE"),
      on_protocol_error: () => this.end("KUBERNETES_STREAM_PROTOCOL_ERROR"),
    });
  }
  connect(ticket: KubernetesStreamTicket) {
    const url = new URL(ticket.websocket_url);
    if (
      url.search ||
      url.hash ||
      url.username ||
      url.password ||
      ticket.protocol_version !== protocol ||
      ticket.session.mode !== "exec" ||
      (url.protocol !== "wss:" &&
        !(
          url.protocol === "ws:" &&
          ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname)
        ))
    )
      throw new Error("Invalid Kubernetes stream endpoint");
    this.callbacks.state("connecting");
    const socket = this.transport.connect(url.toString());
    socket.addEventListener(
      "open",
      () => {
        const value = ticket.ticket;
        ticket.ticket = "";
        this.transport.send(
          JSON.stringify({
            protocol,
            type: "client_hello",
            sequence: 1,
            ticket: value,
            cols: 100,
            rows: 30,
          }),
        );
      },
      { once: true },
    );
  }
  input(data: string) {
    if (this.nonce) this.send({ type: "input", data });
  }
  resize(cols: number, rows: number) {
    if (this.nonce)
      this.send({
        type: "resize",
        cols: Math.max(20, Math.min(500, cols)),
        rows: Math.max(5, Math.min(300, rows)),
      });
  }
  close() {
    if (this.nonce && !this.ended) {
      try {
        this.send({ type: "close" });
      } catch {
        /* transport already closed */
      }
    }
    this.transport.close();
    this.end();
  }
  private send(value: object) {
    this.transport.send(
      JSON.stringify({
        ...value,
        protocol,
        nonce: this.nonce,
        sequence: ++this.sent,
      }),
    );
  }
  private receive(raw: string | ArrayBuffer | Blob) {
    try {
      if (typeof raw !== "string") throw new Error();
      const frame = JSON.parse(raw) as Record<string, unknown>;
      if (frame.protocol !== protocol || frame.sequence !== ++this.received)
        throw new Error();
      if (
        frame.type === "server_ready" &&
        this.received === 1 &&
        frame.mode === "exec" &&
        typeof frame.nonce === "string" &&
        frame.nonce.length >= 24
      ) {
        this.nonce = frame.nonce;
        this.callbacks.state("connected");
        return;
      }
      if (!this.nonce) throw new Error();
      if (frame.type === "output" && typeof frame.data === "string") {
        const bytes = Uint8Array.from(atob(frame.data), (c) => c.charCodeAt(0));
        this.callbacks.output(this.decoder.decode(bytes, { stream: true }));
        return;
      }
      if (frame.type === "session_state") {
        this.end(typeof frame.reason === "string" ? frame.reason : undefined);
        this.transport.close();
        return;
      }
      if (frame.type !== "pong") throw new Error();
    } catch {
      this.end("KUBERNETES_STREAM_PROTOCOL_ERROR");
      this.transport.close();
    }
  }
  private end(reason?: string) {
    if (this.ended) return;
    this.ended = true;
    this.nonce = "";
    const tail = this.decoder.decode();
    if (tail) this.callbacks.output(tail);
    this.callbacks.state("disconnected", reason);
  }
}
