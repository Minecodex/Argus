import { useId, useState, type ReactNode } from "react";
import {
  Check,
  ChevronDown,
  CircleHelp,
  Clock3,
  FileSearch,
  LoaderCircle,
  Pencil,
  Terminal,
  Wrench,
  X,
} from "lucide-react";
import { Button } from "./button";

export type ToolActivityStatus =
  "queued" | "running" | "success" | "failed" | "cancelled" | "unknown";
export type ToolActivityKind = "tool" | "file_read" | "file_change" | "command";

export function ToolActivityState({
  status,
  label,
}: {
  status: ToolActivityStatus;
  label: string;
}) {
  const Icon =
    status === "running"
      ? LoaderCircle
      : status === "success"
        ? Check
        : status === "queued"
          ? Clock3
          : status === "unknown"
            ? CircleHelp
            : X;
  return (
    <span
      className="argus-tool-activity__state"
      data-state={status}
      role="status"
      aria-label={label}
      title={label}
    >
      <Icon
        aria-hidden
        className={status === "running" ? "argus-spin" : undefined}
      />
    </span>
  );
}

/** Flat, accessible activity disclosure; details mount only when requested. */
export function ToolActivityRow({
  title,
  summary,
  kind = "tool",
  status,
  statusLabel,
  duration,
  change,
  wide = false,
  children,
}: {
  title: string;
  summary?: string;
  kind?: ToolActivityKind;
  status: ToolActivityStatus;
  statusLabel: string;
  duration?: string;
  change?: { added: number; removed: number };
  wide?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const Icon =
    kind === "file_change"
      ? Pencil
      : kind === "file_read"
        ? FileSearch
        : kind === "command"
          ? Terminal
          : Wrench;
  return (
    <div
      className="argus-tool-activity"
      data-state={status}
      data-kind={kind}
      data-wide={open && wide ? "true" : undefined}
    >
      <Button
        variant="ghost"
        className="argus-tool-activity__toggle"
        aria-expanded={open}
        aria-controls={id}
        onPress={() => setOpen(!open)}
      >
        <Icon aria-hidden />
        <span className="argus-tool-activity__title">{title}</span>
        {summary && (
          <span className="argus-tool-activity__summary" title={summary}>
            {summary}
          </span>
        )}
        <span className="argus-tool-activity__meta">
          {change && (
            <span className="argus-tool-activity__changes">
              <span className="is-added">+{change.added}</span>
              <span className="is-removed">−{change.removed}</span>
            </span>
          )}
          <span className="argus-tool-activity__duration">
            {duration && <time>{duration}</time>}
          </span>
          <ToolActivityState status={status} label={statusLabel} />
          <ChevronDown aria-hidden className="argus-tool-activity__chevron" />
        </span>
      </Button>
      {open && (
        <div id={id} className="argus-tool-activity__details">
          {children}
        </div>
      )}
    </div>
  );
}

/** The application controls expansion so live updates and pagination keep it. */
export function ToolActivityGroup({
  title,
  summary,
  status,
  statusLabel,
  open,
  onOpenChange,
  children,
}: {
  title: string;
  summary?: string;
  status: ToolActivityStatus;
  statusLabel: string;
  open: boolean;
  onOpenChange(open: boolean): void;
  children: ReactNode;
}) {
  const id = useId();
  return (
    <section
      className="argus-tool-activity-group"
      data-testid="tool-activity-group"
      data-state={status}
      aria-label={title}
    >
      <Button
        variant="ghost"
        className="argus-tool-activity__toggle argus-tool-activity-group__toggle"
        aria-expanded={open}
        aria-controls={id}
        onPress={() => onOpenChange(!open)}
      >
        <Wrench aria-hidden />
        <span className="argus-tool-activity__title">{title}</span>
        {summary && (
          <span className="argus-tool-activity__summary">{summary}</span>
        )}
        <span className="argus-tool-activity__meta">
          <ToolActivityState status={status} label={statusLabel} />
          <ChevronDown aria-hidden className="argus-tool-activity__chevron" />
        </span>
      </Button>
      {open && (
        <div id={id} className="argus-tool-activity-group__items">
          {children}
        </div>
      )}
    </section>
  );
}
