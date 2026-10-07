// The sandbox consumes host tokens; no framework, network or protocol additions.
export const baseStyle = `
:root{color:var(--text-primary);background:var(--bg-surface);font:var(--text-body,var(--font-size-13))/var(--line-height-normal) var(--font-sans)}
*{box-sizing:border-box}body{margin:0;padding:var(--card-padding,var(--space-4))}
h3{font-size:var(--text-body,var(--font-size-13));margin:0 0 var(--field-gap,var(--space-3))}
.argus-template-resource{font:inherit;color:var(--text-primary);background:var(--bg-elevated);border:1px solid var(--border-default);border-radius:var(--control-radius,var(--radius-sm));min-height:var(--control-md,var(--space-8));padding:0 var(--space-3);margin:var(--space-1);cursor:pointer}
.argus-template-resource:hover{background:var(--bg-hover)}.argus-template-resource:focus-visible{outline:2px solid var(--brand-highlight);outline-offset:2px}
table{width:100%;border-collapse:collapse}th,td{text-align:left;border-bottom:1px solid var(--border-subtle);height:var(--record-height,var(--space-8));padding:var(--space-1) var(--space-3);vertical-align:top;overflow-wrap:anywhere}th{color:var(--text-tertiary);font-weight:600;background:var(--bg-elevated)}
dl{display:grid;grid-template-columns:minmax(8rem,1fr) 3fr;gap:var(--field-gap,var(--space-3))}dt{color:var(--text-tertiary)}dd{margin:0;overflow-wrap:anywhere}
pre{white-space:pre-wrap;overflow-wrap:anywhere;font-family:var(--font-mono);border-radius:var(--control-radius,var(--radius-sm));padding:var(--space-3);background:var(--bg-elevated)}
svg{display:block;width:100%;max-height:20rem}p{color:var(--text-secondary)}
@media(prefers-reduced-motion:reduce){*{animation:none!important;transition:none!important}}`;
