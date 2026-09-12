/* eslint-disable @typescript-eslint/no-unused-vars -- ambient constructor declarations are consumed through the default export */
declare module "guacamole-common-js" {
  type MouseEvent = { state: unknown };
  class Tunnel {
    static State: {
      CONNECTING: number;
      OPEN: number;
      CLOSED: number;
      UNSTABLE: number;
    };
    state: number;
    oninstruction: ((opcode: string, parameters: string[]) => void) | null;
    onstatechange: ((state: number) => void) | null;
    onerror: ((status: { message?: string; code?: number }) => void) | null;
    connect(data?: string): void;
    disconnect(): void;
    sendMessage(...elements: unknown[]): void;
    setState(state: number): void;
  }
  class Parser {
    oninstruction: ((opcode: string, parameters: string[]) => void) | null;
    receive(packet: string): void;
  }
  class Display {
    getElement(): HTMLElement;
    getWidth(): number;
    getHeight(): number;
    scale(value: number): void;
  }
  class Client {
    constructor(tunnel: Tunnel);
    connect(data?: string): void;
    disconnect(): void;
    getDisplay(): Display;
    sendMouseState(state: unknown, applyDisplayScale?: boolean): void;
    sendKeyEvent(pressed: number, keysym: number): void;
    sendSize(width: number, height: number): void;
  }
  class Mouse {
    constructor(element: HTMLElement);
    onEach(events: string[], callback: (event: MouseEvent) => void): void;
  }
  class Keyboard {
    constructor(element: HTMLElement | Document);
    onkeydown: ((keysym: number) => boolean) | null;
    onkeyup: ((keysym: number) => void) | null;
    reset(): void;
  }
  class SessionRecording {
    constructor(recording: Blob | Tunnel);
    onload: (() => void) | null;
    onerror: ((message: string) => void) | null;
    onprogress: ((duration: number, bytes: number) => void) | null;
    onplay: (() => void) | null;
    onpause: (() => void) | null;
    onseek: ((position: number) => void) | null;
    connect(data?: string): void;
    disconnect(): void;
    abort(): void;
    play(): void;
    pause(): void;
    seek(position: number, callback?: () => void): void;
    getDisplay(): Display;
    getDuration(): number;
    getPosition(): number;
    isPlaying(): boolean;
  }
  const Guacamole: {
    Tunnel: typeof Tunnel;
    Parser: typeof Parser;
    Client: typeof Client;
    Mouse: typeof Mouse;
    Keyboard: typeof Keyboard;
    SessionRecording: typeof SessionRecording;
  };
  export default Guacamole;
}
