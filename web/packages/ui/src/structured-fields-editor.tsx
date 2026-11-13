import { useState, useEffect, useRef } from "react";
import { Field, Input } from "./form";
import { Button } from "./button";
import { Select } from "./select";
import { useUiText } from "./locale";
import { KeyValueEditor, type KeyValueEntry } from "./key-value-editor";
export type StructuredField = {
  key: string;
  label: string;
  kind?:
    | "text"
    | "number"
    | "int_or_string"
    | "boolean"
    | "select"
    | "map"
    | "list"
    | "object";
  options?: { value: string; label: string }[];
  fields?: StructuredField[];
  itemKind?: "text" | "object";
  required?: boolean;
  secret?: boolean;
  multiline?: boolean;
};
function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}
export function StructuredFieldsEditor({
  fields,
  value,
  onChange,
  disabled = false,
}: {
  fields: StructuredField[];
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  disabled?: boolean;
}) {
  const text = useUiText();
  const set = (key: string, next: unknown) => {
    const result = { ...value };
    if (next === undefined) delete result[key];
    else result[key] = next;
    onChange(result);
  };
  return (
    <div className="argus-structured-fields">
      {fields.map((field) => {
        const current = value[field.key],
          kind = field.kind ?? "text",
          requirement = field.required ? "required" : "optional";
        if (kind === "object")
          return (
            <fieldset key={field.key}>
              <legend>{field.label}</legend>
              <StructuredFieldsEditor
                fields={field.fields ?? []}
                value={record(current)}
                disabled={disabled}
                onChange={(next) => set(field.key, next)}
              />
            </fieldset>
          );
        if (kind === "map")
          return (
            <MapField
              key={field.key}
              field={field}
              value={record(current)}
              disabled={disabled}
              onChange={(next) => set(field.key, next)}
            />
          );
        if (kind === "list") {
          const rows = Array.isArray(current) ? current : [];
          return (
            <fieldset key={field.key} className="argus-structured-fields__list">
              <legend>{field.label}</legend>
              {rows.map((item, index) => (
                <div key={index} className="argus-structured-fields__item">
                  {field.itemKind === "text" ? (
                    <Field
                      label={field.label + " " + (index + 1)}
                      requirement="required"
                    >
                      <Input
                        value={String(item ?? "")}
                        disabled={disabled}
                        onChange={(event) =>
                          set(
                            field.key,
                            rows.map((row, i) =>
                              i === index ? event.target.value : row,
                            ),
                          )
                        }
                      />
                    </Field>
                  ) : (
                    <StructuredFieldsEditor
                      fields={field.fields ?? []}
                      value={record(item)}
                      disabled={disabled}
                      onChange={(next) =>
                        set(
                          field.key,
                          rows.map((row, i) => (i === index ? next : row)),
                        )
                      }
                    />
                  )}
                  <Button
                    variant="ghost"
                    isDisabled={disabled}
                    aria-label={
                      text("移除", "Remove") +
                      " " +
                      field.label +
                      " " +
                      (index + 1)
                    }
                    onPress={() =>
                      set(
                        field.key,
                        rows.filter((_, i) => i !== index),
                      )
                    }
                  >
                    {text("移除", "Remove")}
                  </Button>
                </div>
              ))}
              <Button
                isDisabled={disabled}
                onPress={() =>
                  set(field.key, [...rows, field.itemKind === "text" ? "" : {}])
                }
              >
                {text("添加", "Add") + " " + field.label}
              </Button>
            </fieldset>
          );
        }
        if (kind === "boolean" || kind === "select")
          return (
            <Field
              key={field.key}
              label={field.label}
              requirement={requirement}
            >
              <Select
                value={current === undefined ? "" : String(current)}
                disabled={disabled}
                options={[
                  { value: "", label: text("使用默认值", "Use default") },
                  ...(kind === "boolean"
                    ? [
                        { value: "true", label: text("是", "Yes") },
                        { value: "false", label: text("否", "No") },
                      ]
                    : (field.options ?? [])),
                ]}
                onValueChange={(next) =>
                  set(
                    field.key,
                    next === ""
                      ? undefined
                      : kind === "boolean"
                        ? next === "true"
                        : next,
                  )
                }
              />
            </Field>
          );
        return (
          <Field key={field.key} label={field.label} requirement={requirement}>
            <Input
              type={
                field.secret
                  ? "password"
                  : kind === "number"
                    ? "number"
                    : "text"
              }
              value={String(current ?? "")}
              disabled={disabled}
              onChange={(event) =>
                set(
                  field.key,
                  event.target.value === ""
                    ? undefined
                    : kind === "number" ||
                        (kind === "int_or_string" &&
                          /^\d+$/.test(event.target.value))
                      ? Number(event.target.value)
                      : event.target.value,
                )
              }
            />
          </Field>
        );
      })}
    </div>
  );
}

function MapField({
  field,
  value,
  onChange,
  disabled,
}: {
  field: StructuredField;
  value: Record<string, unknown>;
  onChange: (value: Record<string, string>) => void;
  disabled: boolean;
}) {
  const toEntries = (input: Record<string, unknown>) =>
    Object.entries(input).map(([key, v]) => ({
      id: crypto.randomUUID(),
      key,
      value: String(v ?? ""),
    }));
  const [entries, setEntries] = useState<KeyValueEntry[]>(() =>
    toEntries(value),
  );
  const last = useRef(JSON.stringify(value));
  useEffect(() => {
    if (JSON.stringify(value) !== last.current) {
      last.current = JSON.stringify(value);
      setEntries(toEntries(value));
    }
  }, [value]);
  return (
    <KeyValueEditor
      label={field.label}
      entries={entries}
      disabled={disabled}
      protectedValues={field.secret}
      multiline={field.multiline}
      onChange={(next) => {
        setEntries(next);
        if (
          next.some((row) => !row.key) ||
          new Set(next.map((row) => row.key)).size !== next.length
        )
          return;
        const result = Object.fromEntries(
          next.map((row) => [row.key, row.value]),
        );
        last.current = JSON.stringify(result);
        onChange(result);
      }}
    />
  );
}
