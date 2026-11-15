/** Asciicast v2 events carried by remote-session recordings. */
export type TerminalPlayerEvent = {
  time: number;
  type: "i" | "o" | "r" | "m";
  data: unknown;
};

export const INITIAL_TERMINAL_SIZE = { cols: 80, rows: 24 };

export function parseTerminalResize(data: unknown) {
  if (typeof data !== "string") return null;
  const match = /^(\d+)x(\d+)$/.exec(data);
  if (!match) return null;
  const cols = Number(match[1]),
    rows = Number(match[2]);
  return cols >= 2 && cols <= 500 && rows >= 2 && rows <= 500
    ? { cols, rows }
    : null;
}

export function terminalRecordingExtent(
  events: readonly TerminalPlayerEvent[],
) {
  const extent = { ...INITIAL_TERMINAL_SIZE };
  for (const event of events) {
    const size = event.type === "r" ? parseTerminalResize(event.data) : null;
    if (size) {
      extent.cols = Math.max(extent.cols, size.cols);
      extent.rows = Math.max(extent.rows, size.rows);
    }
  }
  return extent;
}

type ReplayTerminal = {
  reset(): void;
  resize(cols: number, rows: number): void;
  write(data: string, complete: () => void): void;
};

/** Serialize asynchronous writes with resize/reset. A newer seek supersedes the
 * pending replay, waiting for the single in-flight write before clearing it. */
export class TerminalReplay {
  private cursor = 0;
  private epoch = 0;
  private draining = false;
  private disposed = false;
  private pending: { target: number; reset: boolean; epoch: number } | null =
    null;

  constructor(
    private terminal: ReplayTerminal,
    private events: readonly TerminalPlayerEvent[],
    private rendered: (time: number) => void,
  ) {}

  setEvents(events: readonly TerminalPlayerEvent[]) {
    this.events = events;
    this.seek(0);
  }

  seek(target: number) {
    this.epoch++;
    this.schedule(target, true);
  }

  advance(target: number) {
    this.schedule(target, false);
  }

  dispose() {
    this.disposed = true;
    this.epoch++;
    this.pending = null;
  }

  private schedule(target: number, reset: boolean) {
    if (this.disposed) return;
    this.pending = {
      target,
      reset: reset || Boolean(this.pending?.reset),
      epoch: this.epoch,
    };
    if (!this.draining) void this.drain();
  }

  private async drain() {
    this.draining = true;
    try {
      while (this.pending && !this.disposed) {
        const request = this.pending;
        this.pending = null;
        const events = this.events;
        if (request.reset) {
          this.terminal.reset();
          this.terminal.resize(
            INITIAL_TERMINAL_SIZE.cols,
            INITIAL_TERMINAL_SIZE.rows,
          );
          this.cursor = 0;
        }
        while (
          !this.disposed &&
          request.epoch === this.epoch &&
          this.cursor < events.length &&
          events[this.cursor]!.time <= request.target
        ) {
          const event = events[this.cursor]!;
          if (event.type === "r") {
            const size = parseTerminalResize(event.data);
            if (size) this.terminal.resize(size.cols, size.rows);
            this.cursor++;
            continue;
          }
          const chunks: string[] = [];
          let length = 0;
          // Batch output without crossing a resize boundary or queuing the
          // whole recording in xterm's asynchronous parser at once.
          while (
            this.cursor < events.length &&
            events[this.cursor]!.time <= request.target &&
            events[this.cursor]!.type !== "r" &&
            length < 65536
          ) {
            const next = events[this.cursor++]!;
            if (next.type === "o" && typeof next.data === "string") {
              chunks.push(next.data);
              length += next.data.length;
            }
          }
          if (chunks.length)
            await new Promise<void>((resolve) =>
              this.terminal.write(chunks.join(""), resolve),
            );
        }
        if (!this.disposed && request.epoch === this.epoch)
          this.rendered(request.target);
      }
    } finally {
      this.draining = false;
    }
  }
}
