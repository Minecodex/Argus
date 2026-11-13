import { Button } from "./button";
import { Input, Field, Textarea } from "./form";
import { useUiText } from "./locale";
export type KeyValueEntry = { id: string; key: string; value: string };
export function KeyValueEditor({
  label,
  entries,
  onChange,
  protectedValues = false,
  multiline = false,
  disabled = false,
}: {
  label: string;
  entries: KeyValueEntry[];
  onChange: (values: KeyValueEntry[]) => void;
  protectedValues?: boolean;
  multiline?: boolean;
  disabled?: boolean;
}) {
  const text = useUiText();
  return (
    <fieldset className="argus-key-value-editor">
      <legend>{label}</legend>
      {entries.map((entry, index) => (
        <div className="argus-key-value-editor__row" key={entry.id}>
          <Field
            label={text("键", "Key") + " " + (index + 1)}
            requirement="required"
          >
            <Input
              required
              value={entry.key}
              ref={(node) => {
                node?.setCustomValidity(
                  entries.some(
                    (other) => other.id !== entry.id && other.key === entry.key,
                  )
                    ? text("键不能重复", "Keys must be unique")
                    : "",
                );
              }}
              disabled={disabled}
              onChange={(event) =>
                onChange(
                  entries.map((item) =>
                    item.id === entry.id
                      ? { ...item, key: event.target.value }
                      : item,
                  ),
                )
              }
            />
          </Field>
          <Field
            label={text("值", "Value") + " " + (index + 1)}
            requirement="optional"
          >
            {(multiline || /[\r\n]/.test(entry.value)) && !protectedValues ? (
              <Textarea
                rows={Math.min(
                  8,
                  Math.max(2, entry.value.split(/\r?\n/).length),
                )}
                value={entry.value}
                disabled={disabled}
                spellCheck={false}
                onChange={(event) =>
                  onChange(
                    entries.map((item) =>
                      item.id === entry.id
                        ? { ...item, value: event.target.value }
                        : item,
                    ),
                  )
                }
              />
            ) : (
              <Input
                type={protectedValues ? "password" : "text"}
                value={entry.value}
                disabled={disabled}
                onChange={(event) =>
                  onChange(
                    entries.map((item) =>
                      item.id === entry.id
                        ? { ...item, value: event.target.value }
                        : item,
                    ),
                  )
                }
              />
            )}
          </Field>
          <Button
            variant="ghost"
            isDisabled={disabled}
            aria-label={text("移除此项", "Remove entry") + " " + (index + 1)}
            onPress={() =>
              onChange(entries.filter((item) => item.id !== entry.id))
            }
          >
            {text("移除", "Remove")}
          </Button>
        </div>
      ))}
      <Button
        isDisabled={disabled}
        onPress={() =>
          onChange([
            ...entries,
            { id: crypto.randomUUID(), key: "", value: "" },
          ])
        }
      >
        {text("添加一项", "Add entry")}
      </Button>
    </fieldset>
  );
}
