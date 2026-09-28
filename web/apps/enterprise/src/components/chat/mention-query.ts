/** Keep multiword resource names searchable without spanning message lines. */
export function mentionQuery(value: string, caret: number) {
  const match = /(?:^|\s)@([^@\r\n]{0,256})$/.exec(value.slice(0, caret));
  return match
    ? { start: caret - match[1]!.length - 1, end: caret, query: match[1]! }
    : null;
}
