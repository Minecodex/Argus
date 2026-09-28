import { useEffect, useRef, useState } from "react";
import { Button } from "./button";
import { Input } from "./form";
import { useUiText } from "./locale";

export type TelemetryCatalogPage = {
  fields: { name: string; type: string }[];
  values: string[];
  has_more: boolean;
  next_cursor?: string;
};
export function TelemetryFieldExplorer({
  scope,
  load,
  onUse,
  canUse,
}: {
  scope: string;
  load: (
    request: {
      kind: "fields" | "values";
      field?: string;
      search: string;
      cursor?: string;
    },
    signal: AbortSignal,
  ) => Promise<TelemetryCatalogPage>;
  onUse?: (field: string, value: string) => void;
  canUse?: (field: string) => boolean;
}) {
  const text = useUiText();
  const [fields, setFields] = useState<TelemetryCatalogPage["fields"]>([]);
  const [values, setValues] = useState<string[]>([]);
  const [field, setField] = useState("");
  const [search, setSearch] = useState("");
  const [valueSearch, setValueSearch] = useState("");
  const [fieldCursor, setFieldCursor] = useState<string>();
  const [valueCursor, setValueCursor] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  async function query(
    kind: "fields" | "values",
    selected = field,
    cursor?: string,
    searchOverride?: string,
  ) {
    pending.current?.abort();
    const controller = new AbortController();
    pending.current = controller;
    setBusy(true);
    setError(false);
    if (kind === "fields" && !cursor) {
      setFields([]);
      setField("");
      setValues([]);
      setFieldCursor(undefined);
      setValueCursor(undefined);
    }
    if (kind === "values" && !cursor) {
      setField(selected);
      setValues([]);
      setValueCursor(undefined);
    }
    try {
      const page = await load(
        {
          kind,
          field: kind === "values" ? selected : undefined,
          search: searchOverride ?? (kind === "fields" ? search : valueSearch),
          cursor,
        },
        controller.signal,
      );
      if (controller.signal.aborted) return;
      const next = page.has_more ? page.next_cursor : undefined;
      if (kind === "fields") {
        setFields((previous) =>
          cursor ? [...previous, ...page.fields] : page.fields,
        );
        setFieldCursor(next);
        setLoaded(true);
      } else {
        setValues((previous) =>
          cursor ? [...previous, ...page.values] : page.values,
        );
        setValueCursor(next);
      }
    } catch {
      if (!controller.signal.aborted) setError(true);
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <section
      className="argus-field-explorer"
      aria-label={text("字段探索", "Field explorer")}
      aria-busy={busy}
    >
      <p>{scope}</p>
      <div className="argus-field-explorer__search">
        <Input
          aria-label={text("搜索字段", "Search fields")}
          value={search}
          disabled={busy}
          onChange={(e) => {
            setSearch(e.target.value);
            setFieldCursor(undefined);
          }}
        />
        <Button disabled={busy} onClick={() => void query("fields")}>
          {text("读取字段", "Load fields")}
        </Button>
      </div>
      {error && (
        <p role="alert">
          {text(
            "读取失败，请重试。未确认字段或选值消失。",
            "Could not load data. Retry; absence has not been confirmed.",
          )}
        </p>
      )}
      {loaded && !busy && !error && !fields.length && (
        <p>
          {text("此范围内没有匹配字段", "No matching fields in this scope")}
        </p>
      )}
      <div className="argus-field-explorer__columns">
        <div>
          <ul className="argus-field-explorer__list">
            {fields.map((item) => (
              <li key={item.name}>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  aria-pressed={field === item.name}
                  onClick={() => {
                    setValueSearch("");
                    void query("values", item.name, undefined, "");
                  }}
                >
                  {item.name}
                </Button>
                <span>{item.type}</span>
              </li>
            ))}
          </ul>
          {fieldCursor && (
            <Button
              disabled={busy}
              onClick={() => void query("fields", "", fieldCursor)}
            >
              {text("更多字段", "More fields")}
            </Button>
          )}
        </div>
        {field && (
          <div>
            <strong>{field}</strong>
            <div className="argus-field-explorer__search">
              <Input
                aria-label={text("搜索字段值", "Search field values")}
                value={valueSearch}
                disabled={busy}
                onChange={(e) => {
                  setValueSearch(e.target.value);
                  setValueCursor(undefined);
                }}
              />
              <Button disabled={busy} onClick={() => void query("values")}>
                {text("读取字段值", "Load field values")}
              </Button>
            </div>
            {!busy && !error && !values.length && (
              <p>{text("没有匹配的非空值", "No matching non-empty values")}</p>
            )}
            <ul className="argus-field-explorer__list">
              {values.map((value) => (
                <li key={value}>
                  <code>{value}</code>
                  {onUse && (!canUse || canUse(field)) && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy}
                      aria-label={text(
                        `添加条件 ${field} = ${value}`,
                        `Add filter ${field} = ${value}`,
                      )}
                      onClick={() => onUse(field, value)}
                    >
                      {text("添加条件", "Add filter")}
                    </Button>
                  )}
                </li>
              ))}
            </ul>
            {valueCursor && (
              <Button
                disabled={busy}
                onClick={() => void query("values", field, valueCursor)}
              >
                {text("更多字段值", "More field values")}
              </Button>
            )}
          </div>
        )}
      </div>
    </section>
  );
}
