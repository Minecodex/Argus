import { Button } from "../button";
import { useEffect, useRef, useState } from "react";
import { LoaderCircle } from "lucide-react";
import { createTemplateHost } from "./host";
import type {
  TemplateContext,
  TemplatePresentation,
  TemplateResource,
  TemplateReadInput,
  TemplateReadResult,
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
  onRead,
  dataVersion,
  retryLabel,
  layout = "preview",
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
  onRead?(input: TemplateReadInput): Promise<TemplateReadResult>;
  dataVersion?: number;
  retryLabel?: string;
  /** Document layouts grow to the measured content height without a viewport cap. */
  layout?: "preview" | "document";
}) {
  const iframe = useRef<HTMLIFrameElement>(null);
  const host = useRef<ReturnType<typeof createTemplateHost> | undefined>(
    undefined,
  );
  const [status, setStatus] = useState("loading");
  const [height, setHeight] = useState(240);
  const [expanded, setExpanded] = useState(false);
  const [retry, setRetry] = useState(0);
  const callbacks = useRef({ onResource, onResult, onRead });
  callbacks.current = { onResource, onResult, onRead };
  const context = (): TemplateContext => {
    const computed = getComputedStyle(document.documentElement);
    const tokens: Record<string, string> = {};
    for (const key of Array.from(computed))
      if (
        /^--(?:bg|text|font|space|radius|shadow|accent|success|warning|danger|info|border|brand|surface|control|card|field|action|page|section|record|line-height|template|chart|dashboard|scrollbar)(?:-|$)/.test(
          key,
        )
      )
        tokens[key] = computed.getPropertyValue(key).trim();
    return { locale, color_scheme: colorScheme, layout, tokens };
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
        onRead: (input) =>
          callbacks.current.onRead?.(input) ??
          Promise.reject(new Error("PRESENTATION_READ_UNAVAILABLE")),
      });
    } catch {
      setStatus("failed");
    }
    return () => {
      host.current?.destroy();
      host.current = undefined;
    };
  }, [origin, presentation, retry]);
  useEffect(() => {
    host.current?.setContext(currentContext.current);
  }, [locale, colorScheme, layout]);
  useEffect(() => {
    host.current?.notifyData();
  }, [dataVersion]);
  return (
    <section className="argus-tool-presentation" aria-label={title}>
      {status === "loading" && (
        <div className="argus-tool-presentation__placeholder" role="status">
          <LoaderCircle className="argus-spin" aria-hidden size={24} />
          <p>{loadingLabel}</p>
        </div>
      )}
      {status === "failed" && (
        <div className="argus-tool-presentation__placeholder" role="alert">
          <p>{failureLabel}</p>
          {retryLabel && (
            <Button onPress={() => setRetry((value) => value + 1)}>
              {retryLabel}
            </Button>
          )}
        </div>
      )}
      <iframe
        ref={iframe}
        title={title}
        className="argus-tool-presentation__frame"
        hidden={status !== "ready"}
        style={{
          height:
            layout === "document" || expanded ? height : Math.min(height, 480),
        }}
      />
      {layout === "preview" && status === "ready" && height > 480 && (
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
