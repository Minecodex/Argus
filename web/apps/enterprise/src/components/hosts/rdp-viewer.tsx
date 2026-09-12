import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import Guacamole from "guacamole-common-js";

import type { SessionTicketResult } from "@argus/api-client";
import { Alert, Button, Card, CardContent, CardHeader } from "@argus/ui";

function encodeInstruction(elements: unknown[]): string {
  return (
    elements
      .map((element) => {
        const value = String(element ?? "");
        return `${new TextEncoder().encode(value).length}.${value}`;
      })
      .join(",") + ";"
  );
}

export function RDPViewer({
  ticket,
  onClose,
}: {
  ticket: SessionTicketResult;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  const [error, setError] = useState("");
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    const root = container.current;
    if (!root) return;
    const tunnel = new Guacamole.Tunnel();
    const parser = new Guacamole.Parser();
    const client = new Guacamole.Client(tunnel);
    const display = client.getDisplay();
    const socket = new WebSocket(ticket.websocket_url);
    const nonce = crypto.randomUUID().replaceAll("-", "");
    let authenticated = false;
    let disposed = false;
    let enrollmentTicket: string | undefined = ticket.ticket;

    parser.oninstruction = (opcode, parameters) =>
      tunnel.oninstruction?.(opcode, parameters);
    tunnel.sendMessage = (...elements) => {
      if (socket.readyState === WebSocket.OPEN && authenticated)
        socket.send(encodeInstruction(elements));
    };
    tunnel.connect = () => {
      tunnel.setState(Guacamole.Tunnel.State.CONNECTING);
    };
    tunnel.disconnect = () => {
      if (socket.readyState === WebSocket.OPEN)
        socket.close(1000, "client_close");
      tunnel.setState(Guacamole.Tunnel.State.CLOSED);
    };
    tunnel.onerror = (status) =>
      setError(status.message ?? `RDP error ${status.code ?? ""}`);
    socket.addEventListener("open", () => {
      const value = enrollmentTicket;
      enrollmentTicket = undefined;
      socket.send(
        JSON.stringify({
          protocol: "argus.remote_access/v1",
          type: "client_hello",
          sequence: 1,
          ticket: value,
          nonce,
          cols: 1000,
          rows: 500,
        }),
      );
    });
    socket.addEventListener("message", (event) => {
      if (typeof event.data !== "string") {
        socket.close(1008, "binary_frame_rejected");
        return;
      }
      if (!authenticated) {
        try {
          const ready = JSON.parse(event.data) as {
            protocol?: string;
            type?: string;
            mode?: string;
            nonce?: string;
          };
          if (
            ready.protocol !== "argus.remote_access/v1" ||
            ready.type !== "server_ready" ||
            ready.mode !== "rdp_guacamole" ||
            ready.nonce !== nonce
          ) {
            throw new Error(t("hosts.terminal.rdpHandshakeInvalid"));
          }
          authenticated = true;
          tunnel.setState(Guacamole.Tunnel.State.OPEN);
          setConnected(true);
        } catch (cause) {
          setError(
            cause instanceof Error
              ? cause.message
              : t("hosts.terminal.rdpHandshakeFailed"),
          );
          socket.close(1008, "invalid_handshake");
        }
        return;
      }
      parser.receive(event.data);
    });
    socket.addEventListener("close", (event) => {
      tunnel.setState(Guacamole.Tunnel.State.CLOSED);
      setConnected(false);
      if (!disposed && event.code !== 1000)
        setError(event.reason || t("hosts.terminal.rdpConnectionClosed"));
    });
    socket.addEventListener("error", () =>
      setError(t("hosts.terminal.rdpWebSocketFailed")),
    );

    root.replaceChildren(display.getElement());
    const mouse = new Guacamole.Mouse(display.getElement());
    mouse.onEach(["mousedown", "mousemove", "mouseup"], (event) =>
      client.sendMouseState(event.state, true),
    );
    const keyboard = new Guacamole.Keyboard(document);
    keyboard.onkeydown = (keysym) => {
      client.sendKeyEvent(1, keysym);
      return false;
    };
    keyboard.onkeyup = (keysym) => client.sendKeyEvent(0, keysym);
    const resize = () => {
      const width = Math.max(320, Math.min(1000, root.clientWidth));
      const height = Math.max(200, Math.min(500, root.clientHeight));
      const displayWidth = Math.max(1, display.getWidth());
      const displayHeight = Math.max(1, display.getHeight());
      display.scale(Math.min(width / displayWidth, height / displayHeight));
      if (authenticated) client.sendSize(width, height);
    };
    const observer = new ResizeObserver(resize);
    observer.observe(root);
    client.connect();
    resize();
    return () => {
      disposed = true;
      observer.disconnect();
      keyboard.reset();
      client.disconnect();
      root.replaceChildren();
    };
  }, [t, ticket]);

  return (
    <Card className="argus-rdp-viewer">
      <CardHeader
        action={
          <Button onClick={onClose} size="sm" variant="secondary">
            {t("hosts.terminal.rdpClose")}
          </Button>
        }
        title={
          connected
            ? t("hosts.terminal.rdpConnected")
            : t("hosts.terminal.rdpConnecting")
        }
      />
      <CardContent>
        {error && (
          <Alert
            description={error}
            title={t("hosts.terminal.rdpFailed")}
            tone="danger"
          />
        )}
        <div
          className="argus-rdp-viewer__canvas"
          ref={container}
          tabIndex={0}
        />
      </CardContent>
    </Card>
  );
}
