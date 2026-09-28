import type { ReactNode } from "react";
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
          <Textarea
            className="argus-query-editor__code"
            aria-label={text("查询语句", "Query expression")}
            value={expression ?? ""}
            onChange={(e) => onExpressionChange?.(e.target.value)}
            spellCheck={false}
            rows={7}
          />
          {language === "kql" && (
            <Textarea
              className="argus-query-editor__code"
              aria-label={text("处理管道", "Query pipeline")}
              value={pipeline ?? ""}
              onChange={(e) => onPipelineChange?.(e.target.value)}
              spellCheck={false}
              rows={2}
            />
          )}
        </>
      )}
    </div>
  );
}
