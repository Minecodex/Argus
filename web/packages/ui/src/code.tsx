import { Copy } from "lucide-react";
import { type ReactNode } from "react";
import { Button } from "./button";
import { cx } from "./lib";
import { useUiText } from "./locale";

export function CodeBlock({
  code,
  language = "text",
  copyable = true,
  label,
}: {
  code: string;
  language?: string;
  copyable?: boolean;
  label?: string;
}) {
  const text = useUiText();
  return (
    <div className="argus-code">
      <div className="argus-code__head">
        <span>{label ?? language}</span>
        {copyable && (
          <Button
            aria-label={text("复制代码", "Copy code")}
            onPress={() => navigator.clipboard?.writeText(code)}
            isIconOnly
            variant="ghost"
          >
            <Copy size={13} />
          </Button>
        )}
      </div>
      <pre
        tabIndex={0}
        role="region"
        aria-label={label ?? text("代码", "Code")}
      >
        <code>{code}</code>
      </pre>
    </div>
  );
}

export function DiffViewer({
  lines,
}: {
  lines: Array<{ type: "add" | "remove" | "context"; content: string }>;
}) {
  const text = useUiText();
  return (
    <div
      className="argus-diff"
      tabIndex={0}
      role="region"
      aria-label={text("变更差异", "Change diff")}
    >
      {lines.map((line, index) => (
        <div className={cx(`is-${line.type}`)} key={index}>
          <span>
            {line.type === "add" ? "+" : line.type === "remove" ? "-" : " "}
          </span>
          <code>{line.content}</code>
        </div>
      ))}
    </div>
  );
}

export function Alert({
  title,
  description,
  tone = "info",
  icon,
}: {
  title: string;
  description: string;
  tone?: "info" | "success" | "warning" | "danger";
  icon?: ReactNode;
}) {
  return (
    <div
      className={cx("argus-alert", `is-${tone}`)}
      role={tone === "danger" ? "alert" : "status"}
    >
      {icon && <span className="argus-alert__icon">{icon}</span>}
      <div>
        <b>{title}</b>
        <p>{description}</p>
      </div>
    </div>
  );
}
