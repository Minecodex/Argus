import { lazy, Suspense } from "react";
import { cx } from "./lib";
const MarkdownContent = lazy(() => import("./markdown-content"));

/** Render untrusted conversation/file text without executing HTML or remote images. */
export function Markdown({
  children,
  className,
}: {
  children: string;
  className?: string;
}) {
  return (
    <div className={cx("argus-markdown", className)}>
      <Suspense
        fallback={<div className="argus-markdown__loading">{children}</div>}
      >
        <MarkdownContent>{children}</MarkdownContent>
      </Suspense>
    </div>
  );
}
