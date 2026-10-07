import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import Guacamole from "guacamole-common-js";

import { Button, RangeSlider, type TerminalPlayerEvent } from "@argus/ui";

export function guacamoleRecordingStream(
  events: readonly TerminalPlayerEvent[],
): string {
  return events
    .filter((event) => event.type === "o" && typeof event.data === "string")
    .map((event) => event.data as string)
    .join("");
}

function clock(milliseconds: number): string {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000));
  const minutes = Math.floor(seconds / 60);
  return `${String(minutes).padStart(2, "0")}:${String(seconds % 60).padStart(2, "0")}`;
}

export function RDPRecordingPlayer({
  events,
  emptyLabel,
}: {
  events: readonly TerminalPlayerEvent[];
  emptyLabel: string;
}) {
  const { t } = useTranslation();
  const stream = useMemo(() => guacamoleRecordingStream(events), [events]);
  const rootRef = useRef<HTMLDivElement>(null);
  const playerRef = useRef<InstanceType<
    typeof Guacamole.SessionRecording
  > | null>(null);
  const [ready, setReady] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [position, setPosition] = useState(0);
  const [duration, setDuration] = useState(0);
  const [error, setError] = useState("");

  useEffect(() => {
    const root = rootRef.current;
    if (!root || stream === "") return;
    const recording = new Guacamole.SessionRecording(
      new Blob([stream], {
        type: "application/vnd.apache.guacamole.recording",
      }),
    );
    playerRef.current = recording;
    const display = recording.getDisplay();
    root.replaceChildren(display.getElement());
    const resize = () => {
      const width = Math.max(320, root.clientWidth);
      const displayWidth = Math.max(1, display.getWidth());
      display.scale(Math.min(1, width / displayWidth));
    };
    const observer = new ResizeObserver(resize);
    observer.observe(root);
    recording.onload = () => {
      setDuration(recording.getDuration());
      setReady(true);
      resize();
    };
    recording.onprogress = (nextDuration) => setDuration(nextDuration);
    recording.onplay = () => setPlaying(true);
    recording.onpause = () => setPlaying(false);
    recording.onseek = (nextPosition) => setPosition(nextPosition);
    recording.onerror = (message) => setError(message);
    return () => {
      observer.disconnect();
      recording.abort();
      root.replaceChildren();
      playerRef.current = null;
    };
  }, [stream]);

  if (stream === "")
    return <div className="argus-rdp-recording__empty">{emptyLabel}</div>;
  return (
    <div className="argus-rdp-recording">
      <div
        aria-label={t("remoteSessions.rdpReplayCanvas")}
        className="argus-rdp-recording__canvas"
        ref={rootRef}
        role="img"
      />
      {error && <div className="argus-rdp-recording__error">{error}</div>}
      <div className="argus-rdp-recording__controls">
        <Button
          isDisabled={!ready}
          onPress={() => {
            const player = playerRef.current;
            if (!player) return;
            if (player.isPlaying()) player.pause();
            else player.play();
          }}
          size="sm"
          variant="secondary"
        >
          {playing
            ? t("remoteSessions.rdpReplayPause")
            : t("remoteSessions.rdpReplayPlay")}
        </Button>
        <RangeSlider
          label={t("remoteSessions.rdpReplayPosition")}
          disabled={!ready || duration <= 0}
          max={Math.max(1, duration)}
          min={0}
          onChange={(next) => {
            setPosition(next);
            playerRef.current?.seek(next);
          }}
          step={100}
          value={Math.min(position, Math.max(1, duration))}
        />
        <span>
          {clock(position)} / {clock(duration)}
        </span>
      </div>
    </div>
  );
}
