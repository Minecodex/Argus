import { Button } from "../button";
import { useEffect, useRef, useState } from "react";
import { createTemplateHost } from "./host";
import type {
  TemplateContext,
  TemplatePresentation,
  TemplateResource,
} from "./protocol";

export function ToolPresentationFrame({
  origin,
  presentation,
  locale,
  colorScheme,
  title,
  loadingLabel,
  failureLabel,
  expandLabel,
  collapseLabel,
  onResource,
  onResult,
}: {
  origin: string;
  presentation: TemplatePresentation;
  locale: "zh-CN" | "en-US";
  colorScheme: "light" | "dark";
  title: string;
  loadingLabel: string;
  failureLabel: string;
  expandLabel: string;
  collapseLabel: string;
  onResource?(ref: TemplateResource): void;
  onResult?(ref: string): void;
}) {
  const iframe = useRef<HTMLIFrameElement>(null);
  const host = useRef<ReturnType<typeof createTemplateHost> | undefined>(
    undefined,
  );
  const [status, setStatus] = useState("loading");
  const [height, setHeight] = useState(240);
  const [expanded, setExpanded] = useState(false);
  const callbacks = useRef({ onResource, onResult });
  callbacks.current = { onResource, onResult };
  const context = (): TemplateContext => {
    const computed = getComputedStyle(document.documentElement);
    const tokens: Record<string, string> = {};
    for (const key of Array.from(computed))
      if (
        /^--(?:bg|text|font|space|radius|shadow|accent|success|warning|danger|info|border|brand|surface|control|card|field|action|page|section|record|line-height)(?:-|$)/.test(
          key,
        )
      )
        tokens[key] = computed.getPropertyValue(key).trim();
    return { locale, color_scheme: colorScheme, tokens };
  };
  const currentContext = useRef(context());
  currentContext.current = context();
  useEffect(() => {
    if (!iframe.current) return;
    setStatus("loading");
    try {
      host.current = createTemplateHost(iframe.current, {
        origin,
        presentation,
        context: currentContext.current,
        onReady: () => setStatus("ready"),
        onError: () => setStatus("failed"),
        onResize: setHeight,
        onResource: (ref) => callbacks.current.onResource?.(ref),
        onResult: (ref) => callbacks.current.onResult?.(ref),
      });
    } catch {
      setStatus("failed");
    }
    return () => {
      host.current?.destroy();
      host.current = undefined;
    };
  }, [origin, presentation]);
  useEffect(() => {
    host.current?.setContext(currentContext.current);
  }, [locale, colorScheme]);
  return (
    <section className="argus-tool-presentation" aria-label={title}>
      {status === "loading" && <p role="status">{loadingLabel}</p>}
      {status === "failed" && <p role="alert">{failureLabel}</p>}
      <iframe
        ref={iframe}
        title={title}
        className="argus-tool-presentation__frame"
        hidden={status === "failed"}
        style={{ height: expanded ? height : Math.min(height, 480) }}
      />
      {status === "ready" && height > 480 && (
        <Button
          variant="ghost"
          className="argus-tool-presentation__toggle"
          type="button"
          onPress={() => setExpanded(!expanded)}
          aria-expanded={expanded}
        >
          {expanded ? collapseLabel : expandLabel}
        </Button>
      )}
    </section>
  );
}
