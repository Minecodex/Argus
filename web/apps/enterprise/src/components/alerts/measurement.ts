/** Keep labels readable without rounding a distinct value onto its threshold. */
export function alertMeasurement(
  value: number | null | undefined,
  unit: string,
  locale: string,
  reference?: number,
) {
  if (value == null || !Number.isFinite(value)) return "—";
  let number = String(value);
  for (const precision of [4, 8, 12, 16]) {
    const format = new Intl.NumberFormat(locale, {
      maximumSignificantDigits: precision,
    });
    number = format.format(value);
    if (
      reference === undefined ||
      value === reference ||
      number !== format.format(reference)
    )
      break;
  }
  return unit ? `${number}${unit === "%" ? "" : " "}${unit}` : number;
}
