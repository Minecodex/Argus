export const FILE_PREVIEW_ROWS = 200;
export const FILE_PREVIEW_COLUMNS = 50;
const CELL_CHARS = 4096;

/** RFC-style quoted fields, escaped quotes and embedded CR/LF; bounded before rendering. */
export function parseDelimited(input: string, delimiter: "," | "\t") {
  const source = input.replace(/^\uFEFF/, "");
  const rows: string[][] = [];
  let row: string[] = [],
    field = "",
    quoted = false,
    closed = false,
    used = false;
  let columnsLimited = false,
    cellsLimited = false,
    rowsLimited = false;
  function append(value: string) {
    if (field.length < CELL_CHARS) field += value;
    else cellsLimited = true;
  }
  function cell() {
    if (row.length < FILE_PREVIEW_COLUMNS) row.push(field);
    else columnsLimited = true;
    field = "";
    closed = false;
  }
  function record() {
    cell();
    if (used || row.some((value) => value !== "")) rows.push(row);
    row = [];
    used = false;
  }
  for (let index = 0; index < source.length; index++) {
    const char = source[index]!;
    if (quoted) {
      if (char === '"') {
        if (source[index + 1] === '"') {
          append('"');
          index++;
        } else {
          quoted = false;
          closed = true;
        }
      } else append(char);
      continue;
    }
    if (char === delimiter) {
      used = true;
      cell();
      continue;
    }
    if (char === "\r" || char === "\n") {
      if (char === "\r" && source[index + 1] === "\n") index++;
      record();
      if (rows.length > FILE_PREVIEW_ROWS && index < source.length - 1) {
        rowsLimited = true;
        break;
      }
      continue;
    }
    if (closed || (char === '"' && field !== ""))
      throw new Error("Malformed delimited record");
    used = true;
    if (char === '"') quoted = true;
    else append(char);
  }
  if (!rowsLimited) {
    if (quoted) throw new Error("Unclosed quoted field");
    if (used || field || row.length) record();
  }
  return { rows, rowsLimited, columnsLimited, cellsLimited };
}

/** Validate JSON, but format its lexical tokens so large integers/duplicates remain exact. */
export function formatJsonPreview(source: string): string {
  source = source.replace(/^\uFEFF/, "");
  JSON.parse(source);
  let result = "",
    depth = 0;
  const tokens =
    source.match(
      /"(?:\\.|[^"\\])*"|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null|[{}[\],:]/g,
    ) ?? [];
  const line = () => {
    result += "\n" + "  ".repeat(depth);
  };
  for (let index = 0; index < tokens.length; index++) {
    const char = tokens[index]!;
    if (char === "{" || char === "[") {
      result += char;
      const next = tokens[index + 1];
      if (depth >= 64) throw new Error("JSON nesting preview budget exceeded");
      if (next !== "}" && next !== "]") {
        depth++;
        line();
      }
    } else if (char === "}" || char === "]") {
      const previous = tokens[index - 1];
      if (previous !== "{" && previous !== "[") {
        depth--;
        line();
      }
      result += char;
    } else if (char === ",") {
      result += char;
      line();
    } else if (char === ":") result += ": ";
    else result += char;
  }
  return result;
}
