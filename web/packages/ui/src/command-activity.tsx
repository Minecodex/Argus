import type { ReactNode } from "react";
import { useState } from "react";
import { Check, ChevronDown, LoaderCircle, Terminal, X } from "lucide-react";
import { Button } from "./button";

export function CommandApproval({
  title,
  summary,
  children,
  busy,
  onConfirm,
  onCancel,
  labels,
}: {
  title: string;
  summary?: string;
  children: ReactNode;
  busy?: boolean;
  onConfirm(): void;
  onCancel(): void;
  labels: { confirm: string; cancel: string };
}) {
  return (
    <section className="argus-command-approval" aria-label={title}>
      <header>
        <Terminal aria-hidden size={16} />
        <strong>{title}</strong>
      </header>
      {summary && <p>{summary}</p>}
      <div className="argus-command-activity__body">{children}</div>
      <footer>
        <Button onPress={onConfirm} isDisabled={busy}>
          {labels.confirm}
        </Button>
        <Button variant="ghost" onPress={onCancel} isDisabled={busy}>
          {labels.cancel}
        </Button>
      </footer>
    </section>
  );
}

/** One ordinary tool invocation; never owns or repositions a plan's timeline. */
export function CommandExecution({
  title,
  summary,
  status,
  state,
  children,
  labels,
}: {
  title: string;
  summary?: string;
  status: "running" | "success" | "failed";
  state: string;
  children: ReactNode;
  labels: { expand: string; collapse: string };
}) {
  const [open, setOpen] = useState(false);
  return (
    <section className="argus-command-execution" data-state={status}>
      <Button
        variant="ghost"
        className="argus-command-execution__toggle"
        aria-expanded={open}
        aria-label={open ? labels.collapse : labels.expand}
        onPress={() => setOpen(!open)}
      >
        <Terminal aria-hidden size={14} />
        <strong>{title}</strong>
        {status === "running" ? (
          <LoaderCircle aria-hidden size={14} className="argus-spin" />
        ) : status === "success" ? (
          <Check aria-hidden size={14} />
        ) : (
          <X aria-hidden size={14} />
        )}
        <span className="argus-command-execution__summary">{summary}</span>
        <span role="status">{state}</span>
        <ChevronDown aria-hidden size={14} />
      </Button>
      {open && <div className="argus-command-activity__body">{children}</div>}
    </section>
  );
}
