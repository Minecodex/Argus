import { lazy, Suspense, type ReactNode } from "react";
const CodeEditor = lazy(() => import("./telemetry-code-editor"));
import { Textarea } from "./form";
import { useUiText } from "./locale";

export function TelemetryQueryEditor({
  language,
  mode,
  expression,
  pipeline,
  builder,
  onExpressionChange,
  onPipelineChange,
}: {
  language: string;
  mode: "builder" | "dsl";
  expression?: string;
  pipeline?: string;
  builder?: ReactNode;
  onExpressionChange?: (value: string) => void;
  onPipelineChange?: (value: string) => void;
}) {
  const text = useUiText();
  return (
    <div className="argus-query-editor">
      <div className="argus-query-editor__heading">
        <strong>{language}</strong>
        <span>
          {mode === "builder"
            ? text("构建器", "Builder")
            : text("查询语句", "Query")}
        </span>
      </div>
      {mode === "builder" ? (
        builder
      ) : (
        <>
          <Suspense
            fallback={
              <Textarea
                aria-label={text("查询语句", "Query expression")}
                value={expression ?? ""}
                onChange={(e) => onExpressionChange?.(e.target.value)}
              />
            }
          >
            <CodeEditor
              language={language}
              label={text("查询语句", "Query expression")}
              value={expression ?? ""}
              onChange={onExpressionChange}
            />
            {language === "kql" && (
              <CodeEditor
                language="kql"
                label={text("处理管道", "Query pipeline")}
                value={pipeline ?? ""}
                onChange={onPipelineChange}
              />
            )}
          </Suspense>
        </>
      )}
    </div>
  );
}
