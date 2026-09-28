import { useEffect, useRef, useState } from "react";
import { Field, Textarea } from "./form";
import { useUiText } from "./locale";

export function JsonField<T extends Record<string, unknown> | string[]>({
  label,
  kind,
  value,
  onChange,
}: {
  label: string;
  kind: "object" | "strings";
  value: T;
  onChange: (value: T) => void;
}) {
  const text = useUiText(),
    ref = useRef<HTMLTextAreaElement>(null),
    canonical = JSON.stringify(value),
    last = useRef(canonical);
  const [input, setInput] = useState(() => JSON.stringify(value, null, 2)),
    [error, setError] = useState("");
  useEffect(() => {
    if (canonical !== last.current) {
      last.current = canonical;
      setInput(JSON.stringify(JSON.parse(canonical), null, 2));
      setError("");
      ref.current?.setCustomValidity("");
    }
  }, [canonical]);
  return (
    <Field label={label} requirement="optional" error={error || undefined}>
      <Textarea
        ref={ref}
        className="argus-query-editor__code"
        value={input}
        rows={3}
        spellCheck={false}
        onChange={(event) => {
          setInput(event.target.value);
          try {
            const parsed: unknown = JSON.parse(
              event.target.value || (kind === "object" ? "{}" : "[]"),
            );
            if (
              kind === "object"
                ? parsed === null ||
                  typeof parsed !== "object" ||
                  Array.isArray(parsed)
                : !Array.isArray(parsed) ||
                  !parsed.every((value) => typeof value === "string")
            )
              throw new Error();
            last.current = JSON.stringify(parsed);
            event.currentTarget.setCustomValidity("");
            setError("");
            onChange(parsed as T);
          } catch {
            const message =
              kind === "object"
                ? text("请输入有效的 JSON 对象。", "Enter a valid JSON object.")
                : text(
                    "请输入 JSON 字符串数组。",
                    "Enter a JSON array of strings.",
                  );
            event.currentTarget.setCustomValidity(message);
            setError(message);
          }
        }}
      />
    </Field>
  );
}
