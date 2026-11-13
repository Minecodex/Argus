/** Human-readable Kubernetes quantities; unknown encodings remain visible. */
export function formatResourceQuantity(
  value: unknown,
  kind: "cpu" | "bytes",
): string {
  if (value === undefined || value === null || value === "") return "—";
  const source = String(value),
    match = /^([+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?)([a-zA-Z]*)$/.exec(
      source,
    );
  if (!match) return source;
  const base = Number(match[1]),
    suffix = match[2] ?? "";
  if (kind === "cpu") {
    const scales: Record<string, number> = {
      "": 1,
      m: 0.001,
      u: 0.000001,
      n: 0.000000001,
    };
    return suffix in scales
      ? (base * scales[suffix]!).toLocaleString(undefined, {
          maximumFractionDigits: 3,
        }) + " CPU"
      : source;
  }
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"],
    binary = /^([KMGTPE])i$/.exec(suffix),
    decimal = /^([kKMGTPE])$/.exec(suffix);
  const scale = binary
    ? 1024 ** ("KMGTPE".indexOf(binary[1]!) + 1)
    : decimal
      ? 1000 ** ("kMGTPE".indexOf(decimal[1]!.replace("K", "k")) + 1)
      : suffix === ""
        ? 1
        : suffix === "m"
          ? 0.001
          : undefined;
  if (scale === undefined) return source;
  const number = base * scale;
  if (!Number.isFinite(number)) return source;
  const exponent = Math.min(
    6,
    Math.max(0, Math.floor(Math.log(Math.abs(number) || 1) / Math.log(1024))),
  );
  return (
    (number / 1024 ** exponent).toLocaleString(undefined, {
      maximumFractionDigits: 1,
    }) +
    " " +
    units[exponent]
  );
}
