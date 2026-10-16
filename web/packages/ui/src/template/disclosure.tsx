import { useId, useRef, useState, type ReactNode } from "react";
import { Check, ChevronDown, LoaderCircle, Wrench, X } from "lucide-react";
import { Button } from "../button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../primitives";

type ResultView = { key: string; label: string; content: ReactNode };

function ResultViews({ views }: { views: ResultView[] }) {
  return (
    <Tabs defaultValue={views[0]?.key} className="argus-tool-result__views">
      <TabsList>
        {views.map((view) => (
          <TabsTrigger key={view.key} value={view.key}>
            {view.label}
          </TabsTrigger>
        ))}
      </TabsList>
      {views.map((view) => (
        <TabsContent key={view.key} value={view.key} keepMounted>
          {view.content}
        </TabsContent>
      ))}
    </Tabs>
  );
}

/** A lazy, accessible result shell. Business summaries and content belong to the caller. */
export function ToolResultDisclosure({
  title,
  summary,
  status,
  statusLabel,
  duration,
  viewLabel,
  collapseLabel,
  children,
  views,
}: {
  title: string;
  summary?: string;
  status?: "running" | "success" | "failed";
  statusLabel?: string;
  duration?: string;
  viewLabel: string;
  collapseLabel: string;
  children?: ReactNode;
  views?: ResultView[];
}) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const close = () => {
    setOpen(false);
    trigger.current?.focus({ preventScroll: true });
    trigger.current?.scrollIntoView?.({ block: "nearest" });
  };
  return (
    <section
      className="argus-tool-result"
      data-open={open}
      data-testid="tool-result"
    >
      <header className="argus-tool-result__header">
        <Wrench aria-hidden size={16} />
        <span className="argus-tool-result__title" title={title}>
          {title}
        </span>
        {status && (
          <span
            className={`argus-tool-result__status is-${status}`}
            role="img"
            aria-label={statusLabel}
          >
            {status === "running" ? (
              <LoaderCircle className="argus-spin" aria-hidden size={14} />
            ) : status === "failed" ? (
              <X aria-hidden size={14} />
            ) : (
              <Check aria-hidden size={14} />
            )}
          </span>
        )}
        <span className="argus-tool-result__summary" title={summary}>
          {summary || statusLabel}
        </span>
        {duration && (
          <time className="argus-tool-result__duration">{duration}</time>
        )}
        <Button
          ref={trigger}
          variant="ghost"
          size="sm"
          aria-label={`${open ? collapseLabel : viewLabel} · ${title}`}
          aria-expanded={open}
          aria-controls={id}
          onPress={() => (open ? close() : setOpen(true))}
        >
          {open ? collapseLabel : viewLabel}
          <ChevronDown aria-hidden size={14} />
        </Button>
      </header>
      {open && (
        <div
          id={id}
          className="argus-tool-result__body"
          role="region"
          aria-label={title}
        >
          {views?.length ? <ResultViews views={views} /> : children}
          <footer className="argus-tool-result__footer">
            <Button variant="ghost" size="sm" onPress={close}>
              {collapseLabel}
            </Button>
          </footer>
        </div>
      )}
    </section>
  );
}
