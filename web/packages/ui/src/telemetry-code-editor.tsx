import { tags as syntaxTokens } from "@lezer/highlight";
import { completionFilters } from "./telemetry-completion-filters";
import {
  EqlSingle,
  Neq,
  EqlRegex,
  NeqRegex,
} from "@prometheus-io/lezer-promql";
import { useEffect, useRef, useState } from "react";
import { EditorState, Prec, type Extension } from "@codemirror/state";
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
} from "@codemirror/view";
import {
  defaultKeymap,
  history,
  historyKeymap,
  indentWithTab,
} from "@codemirror/commands";
import {
  bracketMatching,
  HighlightStyle,
  syntaxHighlighting,
} from "@codemirror/language";
import {
  autocompletion,
  completionKeymap,
  startCompletion,
  acceptCompletion,
  type CompletionContext,
  type Completion,
} from "@codemirror/autocomplete";
import {
  PromQLExtension,
  type PrometheusClient,
} from "@prometheus-io/codemirror-promql";
import { buildSchema } from "graphql";
import { getAutocompleteSuggestions, Position } from "graphql-language-service";
import { useUiText } from "./locale";
import {
  useTelemetryEditorEnvironment,
  type TelemetryCompletionRequest,
} from "./telemetry-editor-context";
import { observationTraceSchema } from "./observation-trace-schema";

const traceSchema = buildSchema(observationTraceSchema);
const kqlWords = [
  "AND",
  "OR",
  "NOT",
  "exists",
  "parse json",
  "parse logfmt",
  "where",
  "unwrap",
  "stats",
  "count()",
  "sum()",
  "avg()",
  "min()",
  "max()",
  "p95()",
  "by",
  "bin(timestamp, 1m)",
  "sort",
  "asc",
  "desc",
  "limit",
];
export default function TelemetryCodeEditor({
  language,
  value,
  onChange,
  label,
}: {
  language: string;
  value: string;
  onChange?: (value: string) => void;
  label: string;
}) {
  const parent = useRef<HTMLDivElement>(null),
    view = useRef<EditorView | null>(null),
    latest = useRef({ value, onChange });
  latest.current = { value, onChange };
  const env = useTelemetryEditorEnvironment(),
    environment = useRef(env);
  environment.current = env;
  const text = useUiText();
  const [status, setStatus] = useState<"" | "partial" | "error">("");
  useEffect(() => {
    if (!parent.current) return;
    let disposed = false,
      remote: PromQLExtension | undefined;
    const controllers = new Set<AbortController>();
    const abort = () => {
      controllers.forEach((c) => c.abort());
      controllers.clear();
    };
    const load = async (input: TelemetryCompletionRequest) => {
      const controller = new AbortController();
      controllers.add(controller);
      try {
        await new Promise<void>((resolve, reject) => {
          const timer = setTimeout(resolve, 180);
          controller.signal.addEventListener(
            "abort",
            () => {
              clearTimeout(timer);
              reject(new DOMException("Aborted", "AbortError"));
            },
            { once: true },
          );
        });
        const result = environment.current.load
          ? await environment.current.load(input, controller.signal)
          : { metrics: [], fields: [], values: [], has_more: false };
        controller.signal.throwIfAborted();
        if (!disposed) setStatus(result.has_more ? "partial" : "");
        return result;
      } catch (error) {
        if (!disposed && !controller.signal.aborted) setStatus("error");
        throw error;
      } finally {
        controllers.delete(controller);
      }
    };
    const lang: Extension[] = [];
    if (language === "promql") {
      const client: PrometheusClient = {
        async metricNames(prefix) {
          return (await load({ kind: "metrics", search: prefix })).metrics.map(
            (m) => m.name,
          );
        },
        async labelNames(metric) {
          return (await load({ kind: "fields", metric })).fields.map(
            (f) => f.name,
          );
        },
        async labelValues(field, metric, matchers) {
          const operators: Record<number, "=" | "!=" | "=~" | "!~"> = {
            [EqlSingle]: "=",
            [Neq]: "!=",
            [EqlRegex]: "=~",
            [NeqRegex]: "!~",
          };
          const constraints =
            matchers?.filter(
              (m) => (m.name !== "__name__" || !metric) && m.name !== field,
            ) ?? [];
          if (constraints.some((m) => !operators[m.type])) {
            setStatus("error");
            return [];
          }
          const state = view.current?.state;
          const before = state?.sliceDoc(0, state.selection.main.head) ?? "";
          const raw = before.match(/(?:=~|!~|!=|=)\s*"((?:\\.|[^"\\])*)$/)?.[1];
          let search: string | undefined;
          try {
            search =
              raw === undefined
                ? undefined
                : (JSON.parse(`"${raw}"`) as string);
          } catch {
            return [];
          }
          return (
            await load({
              kind: "values",
              metric,
              field,
              search,
              filters: constraints.map((m) => ({
                field: m.name,
                operator: operators[m.type]!,
                values: [m.value],
              })),
            })
          ).values;
        },
        async metricMetadata() {
          const data = await load({ kind: "metrics" });
          return Object.fromEntries(
            data.metrics.map((m) => [
              m.name,
              [{ type: m.type, help: m.description || m.unit }],
            ]),
          );
        },
        async series(metric, matchers, field) {
          if (!field) return [];
          return (await client.labelValues(field, metric, matchers)).map(
            (value) => new Map([[field, value]]),
          );
        },
        async flags() {
          return {};
        },
        destroy: abort,
      };
      remote = new PromQLExtension().setComplete({ remote: client });
      // CM catches rejected completions; aborted requests are expected on editing/unmount.
      const strategy = remote.getComplete();
      lang.push(
        ...(remote.activateCompletion(false).asExtension() as Extension[]),
      );
      lang.push(
        autocompletion({
          override: [
            async (c) => {
              try {
                return await strategy.promQL(c);
              } catch {
                return null;
              }
            },
          ],
        }),
      );
    } else {
      const complete = async (context: CompletionContext) => {
        const word = context.matchBefore(/[\w.-]+/),
          from = word?.from ?? context.pos;
        if (!word && !context.explicit) return null;
        const before = context.state.sliceDoc(0, context.pos);
        let options: Completion[] = [];
        if (language === "skywalking_graphql") {
          const line = context.state.doc.lineAt(context.pos);
          options = getAutocompleteSuggestions(
            traceSchema,
            context.state.doc.toString(),
            new Position(line.number - 1, context.pos - line.from),
          )
            .filter((x) => !x.label.startsWith("__"))
            .map((x) => ({
              label: x.label,
              detail: typeof x.detail === "string" ? x.detail : undefined,
              type: "property",
            }));
        } else {
          options = kqlWords.map((label) => ({ label, type: "keyword" }));
        }
        const value = before.match(/([\w.-]+)\s*(?::|=|!=)\s*"([^"\n]*)$/);
        try {
          context.addEventListener("abort", abort, { onDocChange: true });
          if (value) {
            const field =
              language === "skywalking_graphql"
                ? ({
                    serviceName: "service_name",
                    operationName: "operation",
                    traceId: "trace_id",
                    sourceId: "source_id",
                    resourceId: "resource_id",
                    serviceInstanceName:
                      "resource_attributes.service.instance.id",
                  }[value[1]!] ?? value[1]!)
                : value[1]!;
            const filters = completionFilters(
              language,
              before.slice(0, value.index),
            );
            if (!filters) {
              setStatus("error");
              return { from, options };
            }
            const data = await load({
              kind: "values",
              field,
              search: value[2],
              filters,
            });
            return {
              from: context.pos - value[2]!.length,
              options: data.values.map((v) => ({
                label: v,
                apply: JSON.stringify(v).slice(1, -1),
                type: "constant",
              })),
            };
          }
          if (language !== "skywalking_graphql") {
            const data = await load({ kind: "fields", search: word?.text });
            options.push(
              ...data.fields.map((f) => ({
                label: f.name,
                detail: f.type,
                type: "property",
              })),
            );
          }
        } catch {
          if (context.aborted) return null;
        }
        return { from, options };
      };
      lang.push(autocompletion({ override: [complete] }));
    }
    const editor = new EditorView({
      parent: parent.current,
      state: EditorState.create({
        doc: latest.current.value,
        extensions: [
          lineNumbers(),
          history(),
          highlightActiveLine(),
          bracketMatching(),
          syntaxHighlighting(
            HighlightStyle.define([
              { tag: syntaxTokens.keyword, color: "var(--accent-text)" },
              { tag: syntaxTokens.variableName, color: "var(--text-primary)" },
              { tag: syntaxTokens.number, color: "var(--info)" },
              { tag: syntaxTokens.string, color: "var(--success)" },
              { tag: syntaxTokens.operator, color: "var(--text-secondary)" },
              { tag: syntaxTokens.comment, color: "var(--text-tertiary)" },
            ]),
            { fallback: true },
          ),
          ...lang,
          EditorView.lineWrapping,
          EditorView.contentAttributes.of({
            "aria-label": label,
            role: "textbox",
            "aria-multiline": "true",
            spellcheck: "false",
          }),
          Prec.highest(
            keymap.of([
              { key: "Ctrl-Space", run: startCompletion },
              { key: "Mod-Space", run: startCompletion },
              {
                key: "Shift-Enter",
                run: () => {
                  environment.current.onRun?.();
                  return true;
                },
              },
              { key: "Tab", run: acceptCompletion },
            ]),
          ),
          keymap.of([
            ...completionKeymap,
            ...defaultKeymap,
            ...historyKeymap,
            indentWithTab,
          ]),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              abort();
              latest.current.onChange?.(update.state.doc.toString());
            }
          }),
          EditorView.theme({
            "&": {
              backgroundColor: "var(--bg-canvas)",
              color: "var(--text-primary)",
              fontSize: "var(--text-body)",
            },
            ".cm-scroller": { fontFamily: "var(--font-mono)" },
            ".cm-content": { caretColor: "var(--text-primary)" },
            ".cm-gutters": {
              backgroundColor: "var(--bg-canvas)",
              color: "var(--text-tertiary)",
              borderColor: "var(--border-subtle)",
            },
            ".cm-cursor": { borderLeftColor: "var(--text-primary)" },
            ".cm-tooltip": {
              backgroundColor: "var(--bg-elevated)",
              color: "var(--text-primary)",
              borderColor: "var(--border-default)",
            },
            ".cm-activeLine": { backgroundColor: "var(--bg-hover)" },
            "&.cm-focused .cm-selectionBackground, .cm-selectionBackground": {
              backgroundColor: "var(--accent-soft)",
            },
          }),
        ],
      }),
    });
    view.current = editor;
    return () => {
      disposed = true;
      abort();
      remote?.destroy();
      editor.destroy();
      view.current = null;
    };
  }, [language, env.key, label]);
  useEffect(() => {
    const current = view.current;
    if (current && current.state.doc.toString() !== value)
      current.dispatch({
        changes: { from: 0, to: current.state.doc.length, insert: value },
      });
  }, [value]);
  return (
    <div className="argus-telemetry-code">
      <div ref={parent} />
      {status && (
        <small role="status">
          {status === "partial"
            ? text(
                "候选未全部加载，请继续输入缩小范围。",
                "More suggestions exist; keep typing to narrow the search.",
              )
            : text(
                "当前上下文的候选读取失败或条件暂不支持，请重试或调整输入。",
                "Suggestions failed or this context is unsupported. Retry or adjust the input.",
              )}
        </small>
      )}
    </div>
  );
}
