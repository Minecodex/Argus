import { useState, type ReactNode } from "react";
import { CheckCircle2, CircleAlert, Clock3, LoaderCircle } from "lucide-react";
import { Card, CardContent, CardHeader } from "./card";
import { StatusBadge, type StatusBadgeProps } from "./status-badge";
import { Timeline, type TimelineItem } from "./display";

export function OperationDetails({
  title,
  initiallyOpen = false,
  children,
}: {
  title: string;
  initiallyOpen?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(initiallyOpen);
  return (
    <details
      className="argus-operation-details"
      open={open}
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary>{title}</summary>
      {children}
    </details>
  );
}

/** A shared, localized view of observed operation state; never invents percent progress. */
export function OperationPanel({
  title,
  description,
  statusLabel,
  tone = "neutral",
  busy,
  meta,
  actions,
  steps,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  statusLabel: string;
  tone?: StatusBadgeProps["tone"];
  busy?: boolean;
  meta?: ReactNode;
  actions?: ReactNode;
  steps?: TimelineItem[];
  children?: ReactNode;
}) {
  const Icon = busy
    ? LoaderCircle
    : tone === "success"
      ? CheckCircle2
      : tone === "danger" || tone === "warning"
        ? CircleAlert
        : Clock3;
  return (
    <Card className="argus-operation-panel">
      <CardHeader
        title={
          <span className="argus-operation-panel__title">
            <Icon
              aria-hidden
              size={20}
              className={busy ? "argus-spin" : undefined}
            />
            {title}
          </span>
        }
        description={description}
        action={
          <StatusBadge tone={tone} pulse={busy}>
            {statusLabel}
          </StatusBadge>
        }
      />
      {(meta || children || steps?.length || actions) && (
        <CardContent className="argus-operation-panel__body">
          {meta && <div className="argus-operation-panel__meta">{meta}</div>}
          {steps && steps.length > 0 && <Timeline items={steps} />}
          {children}
          {actions && (
            <div className="argus-operation-panel__actions">{actions}</div>
          )}
        </CardContent>
      )}
    </Card>
  );
}
