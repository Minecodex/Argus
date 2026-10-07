export type CandidatePage = {
  values: string[];
  next_cursor?: string;
  complete?: boolean;
  selected_exists?: Record<string, boolean>;
};
export function selectionDisappeared(values: string[], page: CandidatePage) {
  return values.some((value) => {
    const checked = page.selected_exists?.[value];
    return (
      checked === false ||
      (checked === undefined &&
        page.complete === true &&
        !page.next_cursor &&
        !page.values.includes(value))
    );
  });
}
