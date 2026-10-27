import { useState } from "react";
import { Button } from "./button";
import { Textarea } from "./form";
import { SegmentedControl } from "./segmented-control";
import { useUiText } from "./locale";
import {
  formatStructuredDocument,
  parseStructuredDocument,
  StructuredDocumentError,
  type DocumentFormat,
  type StructuredDocumentValue,
} from "./structured-document";

export function StructuredDocumentEditor({
  value,
  onChange,
  label,
  disabled,
  readOnly,
  required,
}: {
  value: StructuredDocumentValue;
  onChange: (value: StructuredDocumentValue) => void;
  label: string;
  disabled?: boolean;
  readOnly?: boolean;
  required?: boolean;
}) {
  const text = useUiText(),
    [error, setError] = useState("");
  const format = (next: DocumentFormat) => {
    try {
      const parsed = parseStructuredDocument(value.source, value.format);
      onChange({
        format: next,
        source: formatStructuredDocument(parsed, next),
      });
      setError("");
    } catch (error) {
      const position =
        error instanceof StructuredDocumentError && error.line
          ? ` (${text("行", "line")} ${error.line}, ${text("列", "column")} ${error.column})`
          : "";
      setError(
        text(
          "请输入有效的单个 YAML 或 JSON 对象。",
          "Enter one valid YAML or JSON object.",
        ) + position,
      );
    }
  };
  return (
    <div className="argus-structured-document">
      <div className="argus-structured-document-controls">
        <SegmentedControl
          label={text("文档格式", "Document format")}
          value={value.format}
          onChange={(value) => {
            if (!disabled) format(value as DocumentFormat);
          }}
          options={[
            { value: "yaml", label: "YAML" },
            { value: "json", label: "JSON" },
          ]}
          disabled={disabled}
          density="compact"
        />
        <Button
          type="button"
          size="sm"
          isDisabled={disabled}
          onPress={() => format(value.format)}
        >
          {text("格式化", "Format document")}
        </Button>
      </div>
      <Textarea
        aria-label={label}
        value={value.source}
        disabled={disabled}
        readOnly={readOnly}
        required={required}
        rows={18}
        spellCheck={false}
        className="argus-structured-document-code"
        onChange={(event) => {
          setError("");
          onChange({ ...value, source: event.target.value });
        }}
      />
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
