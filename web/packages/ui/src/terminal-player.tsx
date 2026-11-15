import { RangeSlider } from "./range-slider";
import { Pause, Play, RotateCcw } from "lucide-react";
import {
  type KeyboardEvent as ReactKeyboardEvent,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Button } from "./button";
import { cx } from "./lib";
import { useUiText } from "./locale";
import {
  TerminalReplay,
  terminalRecordingExtent,
  type TerminalPlayerEvent,
} from "./terminal-replay";
export type { TerminalPlayerEvent } from "./terminal-replay";

const VALID_TYPES = new Set(["i", "o", "r", "m"]);
const SPEEDS = [1, 2, 4, 8] as const;

/**
 * Defensive normalization: the API contract guarantees an event array, but a
 * malformed page must degrade to a shorter replay instead of crashing render.
 */
export function normalizeTerminalPlayerEvents(
  raw: readonly unknown[],
): TerminalPlayerEvent[] {
  const events: TerminalPlayerEvent[] = [];
  for (const item of raw) {
    if (typeof item !== "object" || item === null) continue;
    const candidate = item as {
      time?: unknown;
      type?: unknown;
      data?: unknown;
    };
    const time = Number(candidate.time);
    if (!Number.isFinite(time) || time < 0) continue;
    if (typeof candidate.type !== "string" || !VALID_TYPES.has(candidate.type))
      continue;
    events.push({
      time,
      type: candidate.type as TerminalPlayerEvent["type"],
      data: candidate.data,
    });
  }
  events.sort((a, b) => a.time - b.time);
  return events;
}

function formatClock(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  const mm = String(minutes).padStart(2, "0");
  const ss = String(secs).padStart(2, "0");
  return hours > 0 ? `${hours}:${mm}:${ss}` : `${mm}:${ss}`;
}

/**
 * JumpServer-style session recording player: a terminal emulator driven by a
 * wall-clock scheduler over asciicast events. Output events repaint the screen
 * exactly as the live session did; seeking replays the stream from the start,
 * so the progress bar behaves like a video timeline.
 */
export function TerminalPlayer({
  events,
  loading = false,
  height = 420,
  className,
  emptyLabel,
  loadingLabel,
}: {
  /** Stable identity expected (memoize at the call site); changes restart playback. */
  events: readonly TerminalPlayerEvent[];
  loading?: boolean;
  height?: number;
  className?: string;
  emptyLabel?: string;
  loadingLabel?: string;
}) {
  const text = useUiText();
  const screenRef = useRef<HTMLDivElement>(null);
  const viewportRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<HTMLDivElement>(null);
  const replayRef = useRef<TerminalReplay | null>(null);
  const layoutRef = useRef<(() => void) | null>(null);
  const [ready, setReady] = useState(false);
  const [renderedPosition, setRenderedPosition] = useState(-1);
  const [fitRecording, setFitRecording] = useState(false);
  const fitRef = useRef(fitRecording);
  fitRef.current = fitRecording;
  const extent = useMemo(() => terminalRecordingExtent(events), [events]);
  const extentRef = useRef(extent);
  extentRef.current = extent;
  // The scheduler loop reads live values through refs so it is not rebuilt per tick.
  const positionRef = useRef(0);
  const speedRef = useRef(1);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState<number>(1);
  const [position, setPosition] = useState(0);
  const duration = events.at(-1)?.time ?? 0;
  const eventsRef = useRef(events);
  eventsRef.current = events;
  speedRef.current = speed;

  useEffect(() => {
    const viewport = viewportRef.current;
    const canvas = canvasRef.current;
    const element = stageRef.current;
    if (!viewport || !canvas || !element) return;
    let disposed = false;
    let disposeTerminal: (() => void) | undefined;
    void Promise.all([
      import("@xterm/xterm"),
      import("@xterm/xterm/css/xterm.css"),
    ]).then(([xterm]) => {
      if (disposed) return;
      const styles = getComputedStyle(viewport);
      const terminal = new xterm.Terminal({
        convertEol: false,
        cursorBlink: false,
        disableStdin: true,
        fontFamily: styles.fontFamily,
        fontSize: parseFloat(styles.fontSize),
        // The timeline provides history; a replay shows the recorded viewport.
        scrollback: 0,
        theme: {
          background: styles.getPropertyValue("--surface-terminal").trim(),
          foreground: styles.getPropertyValue("--text-console").trim(),
        },
      });
      terminal.open(element);
      const screen = element.querySelector<HTMLElement>(".xterm-screen")!;
      // One cell size and one extent for the entire recording: a timeline
      // resize must never change display zoom or the top-left origin.
      const cell = {
        width: screen.offsetWidth / terminal.cols,
        height: screen.offsetHeight / terminal.rows,
      };
      const fit = () => {
        const width = screen.offsetWidth;
        const height = screen.offsetHeight;
        if (!width || !height) return;
        const fullWidth = extentRef.current.cols * cell.width;
        const fullHeight = extentRef.current.rows * cell.height;
        const scale = fitRef.current
          ? Math.max(
              0,
              Math.min(
                1,
                viewport.clientWidth / fullWidth,
                viewport.clientHeight / fullHeight,
              ),
            )
          : 1;
        element.style.width = `${width}px`;
        element.style.height = `${height}px`;
        element.style.setProperty("--argus-replay-scale", String(scale));
        canvas.style.width = `${fullWidth * scale}px`;
        canvas.style.height = `${fullHeight * scale}px`;
      };
      const observer = new ResizeObserver(fit);
      observer.observe(viewport);
      observer.observe(screen);
      layoutRef.current = fit;
      fit();
      const replay = new TerminalReplay(
        terminal,
        eventsRef.current,
        setRenderedPosition,
      );
      replayRef.current = replay;
      replay.seek(0);
      setReady(true);
      disposeTerminal = () => {
        observer.disconnect();
        replay.dispose();
        terminal.dispose();
        replayRef.current = null;
        layoutRef.current = null;
      };
    });
    return () => {
      disposed = true;
      disposeTerminal?.();
    };
  }, []);

  // Re-emit from the beginning whenever the loaded event stream changes identity.
  useEffect(() => {
    positionRef.current = 0;
    setPosition(0);
    setRenderedPosition(-1);
    setPlaying(false);
    replayRef.current?.setEvents(events);
    viewportRef.current?.scrollTo(0, 0);
    layoutRef.current?.();
  }, [events]);

  useEffect(() => {
    viewportRef.current?.scrollTo(0, 0);
    layoutRef.current?.();
  }, [fitRecording]);

  useEffect(() => {
    if (!playing || !ready) return;
    let frame = 0;
    let last = performance.now();
    const tick = (now: number) => {
      const next = Math.min(
        duration,
        positionRef.current + ((now - last) / 1000) * speedRef.current,
      );
      last = now;
      replayRef.current?.advance(next);
      positionRef.current = next;
      setPosition(next);
      if (next >= duration) {
        setPlaying(false);
        return;
      }
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [playing, duration, ready]);

  const seek = (target: number) => {
    const clamped = Math.min(Math.max(target, 0), duration);
    setRenderedPosition(-1);
    replayRef.current?.seek(clamped);
    positionRef.current = clamped;
    setPosition(clamped);
    if (clamped >= duration) setPlaying(false);
  };

  const toggle = () => {
    if (!ready || !events.length) return;
    if (duration > 0 && positionRef.current >= duration) {
      seek(0);
      viewportRef.current?.scrollTo(0, 0);
    }
    setPlaying((value) => !value);
  };

  const onKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    // Buttons and the slider own their keyboard input.
    if (event.target !== event.currentTarget) return;
    if (event.key === " " || event.key === "k") {
      event.preventDefault();
      toggle();
    } else if (event.key === "ArrowLeft") {
      event.preventDefault();
      seek(positionRef.current - 5);
    } else if (event.key === "ArrowRight") {
      event.preventDefault();
      seek(positionRef.current + 5);
    }
  };

  const summary = useMemo(
    () => `${formatClock(position)} / ${formatClock(duration)}`,
    [position, duration],
  );

  return (
    <div
      aria-busy={!ready || Math.abs(renderedPosition - position) > 0.00001}
      data-view-mode={fitRecording ? "fit" : "native"}
      className={cx("argus-terminal-player", className)}
      onKeyDown={onKeyDown}
      tabIndex={0}
    >
      <div
        className="argus-terminal-player__screen"
        ref={screenRef}
        style={{ height }}
      >
        <div
          className="argus-terminal-player__viewport"
          ref={viewportRef}
          role="region"
          aria-label={text(
            "录像画面，可滚动查看超出部分",
            "Recording viewport, scroll to explore",
          )}
          tabIndex={0}
        >
          <div className="argus-terminal-player__canvas" ref={canvasRef}>
            <div className="argus-terminal-player__stage" ref={stageRef} />
          </div>
        </div>
      </div>
      {(loading || (!loading && events.length === 0)) && (
        <div className="argus-terminal-player__overlay" role="status">
          {loading
            ? (loadingLabel ?? text("录像加载中…", "Loading recording…"))
            : (emptyLabel ?? text("录像中暂无事件", "No recording events"))}
        </div>
      )}
      <div className="argus-terminal-player__controls">
        <div className="argus-terminal-player__actions">
          <Button
            aria-label={playing ? text("暂停", "Pause") : text("播放", "Play")}
            title={playing ? text("暂停", "Pause") : text("播放", "Play")}
            onPress={toggle}
            isDisabled={!ready || !events.length}
            isIconOnly
            variant="primary"
          >
            {playing ? <Pause size={16} /> : <Play size={16} />}
          </Button>
          <Button
            aria-label={text("从头播放", "Restart")}
            title={text("从头播放", "Restart")}
            onPress={() => {
              setPlaying(false);
              seek(0);
              viewportRef.current?.scrollTo(0, 0);
            }}
            isDisabled={!ready || !events.length}
            isIconOnly
            variant="secondary"
          >
            <RotateCcw size={16} />
          </Button>
        </div>
        <RangeSlider
          label={text("录像进度", "Recording position")}
          valueText={summary}
          max={Math.max(duration, 0.1)}
          min={0}
          onChange={seek}
          step={0.1}
          value={position}
          disabled={!ready || !events.length}
        />
        <div className="argus-terminal-player__timing">
          <span className="argus-terminal-player__clock">{summary}</span>
          {position >= duration &&
            renderedPosition >= duration &&
            duration > 0 && (
              <span className="argus-terminal-player__ended" role="status">
                {text("回放结束", "Replay ended")}
              </span>
            )}
        </div>
        <div
          className="argus-terminal-player__views"
          role="group"
          aria-label={text("画面大小", "Display size")}
        >
          <Button
            variant="ghost"
            aria-pressed={!fitRecording}
            aria-label={text(
              "原始字号，超出部分可滚动查看",
              "Original font size, scroll to explore",
            )}
            onPress={() => setFitRecording(false)}
          >
            100%
          </Button>
          <Button
            variant="ghost"
            aria-pressed={fitRecording}
            onPress={() => setFitRecording(true)}
          >
            {text("适应窗口", "Fit")}
          </Button>
        </div>
        <div
          aria-label={text("倍速", "Speed")}
          className="argus-terminal-player__speeds"
          role="group"
        >
          {SPEEDS.map((value) => (
            <Button
              variant="ghost"
              aria-pressed={speed === value}
              className={cx("argus-terminal-player__speed")}
              key={value}
              onPress={() => setSpeed(value)}
              type="button"
            >
              {value}x
            </Button>
          ))}
        </div>
      </div>
    </div>
  );
}
