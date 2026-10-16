import { CheckCircle2, CircleSlash, Clock, XCircle } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { Button } from "./button";
import { StatusBadge } from "./status-badge";
import { DiffViewer } from "./code";
import { cx } from "./lib";
import { useUiText } from "./locale";

export type PreviewRiskLevel = "read" | "write" | "dangerous" | "critical";

export type PreviewCommitStatus =
  "pending" | "success" | "failed" | "cancelled" | "expired";

export type PreviewAffectedItem = {
  name: string;
  detail?: string;
};

function formatRemaining(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

function toMs(value: Date | string | number): number {
  return value instanceof Date ? value.getTime() : new Date(value).getTime();
}

export function PreviewCommitCard({
  title,
  risk,
  riskLabel,
  children,
  diff,
  affected = [],
  expiresAt,
  status = "pending",
  resultMessage,
  confirming,
  confirmLabel,
  cancelLabel,
  onConfirm,
  onCancel,
  className,
  layout = "card",
}: {
  title: ReactNode;
  risk: PreviewRiskLevel;
  riskLabel?: string;
  /** Preview summary area, rendered between the header and the diff. */
  children?: ReactNode;
  diff?: Array<{ type: "add" | "remove" | "context"; content: string }>;
  affected?: PreviewAffectedItem[];
  /** When passed, a live countdown is shown and the card expires on its own. */
  expiresAt?: Date | string | number;
  status?: PreviewCommitStatus;
  /** Custom message for the result state; falls back to a localized default. */
  resultMessage?: ReactNode;
  confirming?: boolean;
  confirmLabel?: string;
  cancelLabel?: string;
  onConfirm?: () => void;
  onCancel?: () => void;
  className?: string;
  layout?: "card" | "dialog" | "workspace";
}) {
  const text = useUiText();
  const expiresAtMs = expiresAt !== undefined ? toMs(expiresAt) : undefined;
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (expiresAtMs === undefined || status !== "pending") return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [expiresAtMs, status]);

  const locallyExpired =
    expiresAtMs !== undefined && now >= expiresAtMs && status === "pending";
  const effectiveStatus: PreviewCommitStatus = locallyExpired
    ? "expired"
    : status;

  const defaultResult: Record<
    Exclude<PreviewCommitStatus, "pending">,
    string
  > = {
    success: text("执行成功", "Executed successfully"),
    failed: text("执行失败", "Execution failed"),
    cancelled: text("操作已取消", "Operation cancelled"),
    expired: text("预览已过期", "Preview expired"),
  };

  return (
    <section
      className={cx(
        "argus-preview-card",
        layout === "dialog" && "argus-preview-card--dialog",
        layout === "workspace" && "argus-preview-card--workspace",
        effectiveStatus !== "pending" &&
          `is-${effectiveStatus === "success" ? "success" : "muted"}`,
        className,
      )}
    >
      {layout !== "dialog" && (
        <header className="argus-preview-card__header">
          {layout !== "workspace" && (
            <span className={cx("argus-risk-badge", `is-${risk}`)}>
              {riskLabel ?? risk}
            </span>
          )}
          <strong className="argus-preview-card__title">{title}</strong>
          {layout === "workspace" && (
            <StatusBadge
              appearance="flat"
              tone="accent"
              title={
                expiresAtMs !== undefined && effectiveStatus === "pending"
                  ? `${riskLabel ?? risk} · ${formatRemaining(expiresAtMs - now)}`
                  : (riskLabel ?? risk)
              }
            >
              {effectiveStatus === "pending"
                ? text("等待用户确认", "Awaiting your confirmation")
                : defaultResult[effectiveStatus]}
            </StatusBadge>
          )}
          {expiresAtMs !== undefined && effectiveStatus === "pending" && (
            <span
              className={cx(
                "argus-preview-card__countdown",
                layout === "workspace" && "sr-only",
              )}
            >
              <Clock aria-hidden size={12} />
              {formatRemaining(expiresAtMs - now)}
            </span>
          )}
        </header>
      )}

      {children && <div className="argus-preview-card__body">{children}</div>}

      {diff && diff.length > 0 && (
        <div className="argus-preview-card__diff">
          <DiffViewer lines={diff} />
        </div>
      )}

      {affected.length > 0 && (
        <ul className="argus-preview-card__affected">
          {affected.map((item) => (
            <li key={item.name}>
              <span>{item.name}</span>
              {item.detail && <small>{item.detail}</small>}
            </li>
          ))}
        </ul>
      )}

      {effectiveStatus === "pending" ? (
        <footer className="argus-preview-card__footer">
          {layout !== "card" && (
            <span className="argus-preview-card__hint">
              {text(
                "请先核对范围与变更，再确认提交。",
                "Review the scope and changes before confirming.",
              )}
            </span>
          )}
          <Button
            className="argus-preview-card__cancel"
            density={layout !== "card" ? "compact" : "default"}
            size={layout !== "card" ? "lg" : "md"}
            isDisabled={confirming}
            onPress={onCancel}
            variant="secondary"
          >
            {cancelLabel ?? text("取消", "Cancel")}
          </Button>
          <Button
            className="argus-preview-card__confirm"
            density={layout !== "card" ? "compact" : "default"}
            size={layout !== "card" ? "lg" : "md"}
            isPending={confirming}
            onPress={onConfirm}
            variant="primary"
          >
            {confirmLabel ?? text("确认执行", "Confirm")}
          </Button>
        </footer>
      ) : (
        <footer
          className={cx("argus-preview-card__result", `is-${effectiveStatus}`)}
        >
          {effectiveStatus === "success" ? (
            <CheckCircle2 aria-hidden size={15} />
          ) : effectiveStatus === "failed" ? (
            <XCircle aria-hidden size={15} />
          ) : (
            <CircleSlash aria-hidden size={15} />
          )}
          <span>{resultMessage ?? defaultResult[effectiveStatus]}</span>
        </footer>
      )}
    </section>
  );
}
