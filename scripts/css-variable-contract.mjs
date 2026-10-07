/** CSS and static React style keys share one variable contract. Keep fallback references explicit. */
export function declaredCssVariables(sources) {
  const defined = new Set();
  for (const source of sources) {
    const text = withoutComments(source);
    for (const match of text.matchAll(/(--[\w-]+)["']?\s*:/g))
      defined.add(match[1]);
    for (const match of text.matchAll(/@property\s+(--[\w-]+)/g))
      defined.add(match[1]);
  }
  return defined;
}
export function undefinedCssVariables(source, defined, runtime = new Set()) {
  const failures = [];
  const text = withoutComments(source);
  for (const match of text.matchAll(/var\(\s*(--[\w-]+)(\s*,)?/g)) {
    if (!match[2] && !defined.has(match[1]) && !runtime.has(match[1]))
      failures.push({
        name: match[1],
        line: text.slice(0, match.index).split("\n").length,
      });
  }
  return failures;
}
function withoutComments(source) {
  return source.replace(/\/\*[\s\S]*?\*\//g, (comment) =>
    comment.replace(/[^\n]/g, " "),
  );
}
