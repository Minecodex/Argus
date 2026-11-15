import { describe, expect, it, vi } from "vitest";
import {
  TerminalReplay,
  terminalRecordingExtent,
  type TerminalPlayerEvent,
} from "./terminal-replay";

class AsyncTerminal {
  cols = 80;
  rows = 24;
  text = "";
  writes: { data: string; cols: number }[] = [];
  pending: (() => void)[] = [];
  reset() {
    this.text = "";
  }
  resize(cols: number, rows: number) {
    this.cols = cols;
    this.rows = rows;
  }
  write(data: string, complete: () => void) {
    this.pending.push(() => {
      this.text += data;
      this.writes.push({ data, cols: this.cols });
      complete();
    });
  }
  flush() {
    this.pending.shift()!();
  }
}
const events: TerminalPlayerEvent[] = [
  { time: 0, type: "r", data: "80x24" },
  { time: 1, type: "o", data: "wide output" },
  { time: 2, type: "r", data: "20x5" },
  { time: 3, type: "o", data: "narrow output" },
];

describe("recorded terminal ordering", () => {
  it("waits for output parsing before applying the following resize", async () => {
    const terminal = new AsyncTerminal(),
      rendered = vi.fn();
    const replay = new TerminalReplay(terminal, events, rendered);
    replay.advance(3);
    expect(terminal.cols).toBe(80);
    expect(rendered).not.toHaveBeenCalled();
    terminal.flush();
    await vi.waitFor(() => expect(terminal.cols).toBe(20));
    terminal.flush();
    await vi.waitFor(() => expect(rendered).toHaveBeenCalledWith(3));
    expect(terminal.writes).toEqual([
      { data: "wide output", cols: 80 },
      { data: "narrow output", cols: 20 },
    ]);
  });

  it("supersedes rapid seeks without leaking queued old output into the new frame", async () => {
    const terminal = new AsyncTerminal(),
      rendered = vi.fn();
    const replay = new TerminalReplay(terminal, events, rendered);
    replay.advance(3);
    replay.seek(3);
    replay.seek(1);
    terminal.flush();
    await vi.waitFor(() => expect(terminal.pending).toHaveLength(1));
    expect(terminal.text).toBe("");
    expect(terminal.cols).toBe(80);
    terminal.flush();
    await vi.waitFor(() => expect(rendered).toHaveBeenCalledExactlyOnceWith(1));
    expect(terminal.text).toBe("wide output");
    replay.seek(0);
    expect(terminal.text).toBe("");
    expect(terminal.cols).toBe(80);
  });

  it("clears the previous recording after an in-flight write finishes", async () => {
    const terminal = new AsyncTerminal(),
      rendered = vi.fn();
    const replay = new TerminalReplay(terminal, events, rendered);
    replay.advance(3);
    replay.setEvents([
      { time: 0, type: "r", data: "40x10" },
      { time: 0, type: "o", data: "new recording" },
    ]);
    terminal.flush();
    await vi.waitFor(() => expect(terminal.cols).toBe(40));
    terminal.flush();
    await vi.waitFor(() => expect(rendered).toHaveBeenCalledExactlyOnceWith(0));
    expect(terminal.text).toBe("new recording");
  });

  it("uses a bounded extent over the entire recording for stable fit mode", () => {
    expect(
      terminalRecordingExtent([
        ...events,
        { time: 4, type: "r", data: "320x90" },
        { time: 5, type: "r", data: "999999x999999" },
      ]),
    ).toEqual({ cols: 320, rows: 90 });
  });
});
