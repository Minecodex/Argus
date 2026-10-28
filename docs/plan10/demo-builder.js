/* Bounded fixture builders. Production conversion stays in the existing Go compiler. */
(() => {
  const D = window.ARGUS_DEMO;
  const known = new Map();
  const copy = x => structuredClone(x);
  const literal = x => JSON.stringify(String(x));
  function defaults(signal, metric = D.metrics[0].name) {
    if (signal === 'metrics') return { metric, operation: 'recipe', group: '' };
    if (signal === 'logs') return { service: 'checkout-api', level: '', keyword: '', output: 'records', limit: '20' };
    return { service: 'checkout-api', status: '', duration: '', trace: '', limit: '20' };
  }
  function compile(signal, b) {
    let expr;
    if (signal === 'metrics') {
      const m = D.metrics.find(m => m.name === b.metric);
      if (!m || !['recipe','value','avg','sum','min','max','rate'].includes(b.operation)) throw new Error('unsupported metric operation');
      if (b.operation === 'rate' && m.type !== 'Counter') throw new Error('rate requires a Counter');
      const selector = m.name;
      if (b.operation === 'recipe') {
        expr = m.query;
      } else if (b.operation === 'value') expr = selector;
      else if (b.operation === 'rate') expr = `rate(${selector}[5m])`;
      else expr = b.group ? `${b.operation} by (argus_resource_id) (${selector})` : `${b.operation}(${selector})`;
    } else if (signal === 'logs') {
      expr = [`service_name:${literal(b.service)}`, b.level && `severity_text:${literal(b.level)}`, b.keyword && `body:${literal(b.keyword)}`].filter(Boolean).join(' AND ');
      expr += b.output === 'count' ? ' | stats count()' : ` | limit ${Number(b.limit)}`;
    } else {
      const args = [`serviceName: ${literal(b.service)}`, b.status && `status: ${literal(b.status)}`, b.duration && `durationMin: ${Number(b.duration)}`, b.trace && `traceId: ${literal(b.trace)}`, 'pageNum: 1', `pageSize: ${Number(b.limit)}`].filter(Boolean);
      expr = `{ queryTraces(${args.join(', ')}) { total traces { traceId rootService duration status } } }`;
    }
    known.set(signal+'\n'+expr, copy(b));
    if (known.size > 500) known.delete(known.keys().next().value);
    return expr;
  }
  function parse(signal, expr) {
    const b = known.get(signal+'\n'+expr.trim());
    return b ? copy(b) : null;
  }
  D.metrics.forEach(m => {
    compile('metrics',defaults('metrics',m.name));
    compile('metrics',{...defaults('metrics',m.name),operation:'value'});
  });
  compile('logs',defaults('logs')); compile('traces',defaults('traces'));
  // Original directory examples have no explicit log limit; do not silently add one during conversion.
  window.ARGUS_BUILDERS = {defaults,compile,parse};
})();
