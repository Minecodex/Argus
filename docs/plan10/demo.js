(() => {
  'use strict';
  const D = window.ARGUS_DEMO;
  const B = window.ARGUS_BUILDERS;
  const $ = (s, root = document) => root.querySelector(s);
  const $$ = (s, root = document) => [...root.querySelectorAll(s)];
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const t = (zh, en) => S.lang === 'zh' ? zh : en;
  const tr = (pair) => pair[S.lang === 'zh' ? 0 : 1];
  let serial = 0, toastTimer, suggest = null, lastDrawerFocus = null;
  const allResources = () => ({mode:'all_authorized',ids:[]});
  const panel = (signal = 'metrics', metric = D.metrics[0], ready = false) => {
    const builder = B.defaults(signal,metric.name), expr = B.compile(signal,builder);
    return {id:++serial, signal, resourceScope:allResources(), executedScope:allResources(), builder, authoring:'builder', expr, metric:signal === 'metrics' ? metric.name : null, title:signal === 'metrics' ? metric.title : null, status:ready?'ready':'idle', view:'graph', mode:'range', advanced:false, step:'auto', revision:0, executed:expr, executedBuilder:structuredClone(builder), expanded:-1, drafts:{}, conversionError:false};
  };
  const S = { lang:'zh', theme:'light', catalogSignal:'metrics', tab:'query', catalogScope:'all', range:'60', search:'', group:'all', observed:true, drawer:null, picker:null, linked:true,
    panels:[panel('metrics',D.metrics[0],true),panel('metrics',D.metrics[1],true)] };
  const icons = {
    metrics: '<path d="M2 12h4l3-8 5 16 3-8h5"/>', logs: '<path d="M5 4h14v16H5zM8 8h8M8 12h8M8 16h5"/>',
    traces: '<circle cx="5" cy="6" r="2"/><circle cx="19" cy="6" r="2"/><circle cx="12" cy="19" r="2"/><path d="M7 6h10M6 8l5 9M18 8l-5 9"/>',
    grid: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
    box: '<path d="m12 3 9 5v9l-9 5-9-5V8zM3 8l9 5 9-5M12 13v9M7 5l9 5"/>',
    server: '<rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6h.01M7 17h.01M12 6h5M12 17h5"/>',
    play: '<path d="m7 4 13 8-13 8z"/>', plus: '<path d="M12 5v14M5 12h14"/>', close: '<path d="m6 6 12 12M6 18 18 6"/>',
    copy: '<rect x="8" y="8" width="13" height="13" rx="2"/><path d="M16 8V3H3v13h5"/>', up: '<path d="m6 14 6-6 6 6"/>', down: '<path d="m6 10 6 6 6-6"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>', book: '<path d="M12 5c-3-2-6-2-10-1v15c4-1 7-1 10 1 3-2 6-2 10-1V4c-4-1-7-1-10 1zM12 5v15"/>',
    moon: '<path d="M20 15A9 9 0 0 1 9 3a9 9 0 1 0 11 12z"/>',
    settings: '<path d="M4 7h16M4 17h16M8 4v6M16 14v6"/>', link: '<path d="m10 14 4-4M8 16l-2 2a4 4 0 0 1-6-6l5-5a4 4 0 0 1 6 0M16 8l2-2a4 4 0 0 1 6 6l-5 5a4 4 0 0 1-6 0"/>',
    check: '<path d="m5 12 4 4L20 5"/>', search: '<circle cx="10" cy="10" r="6"/><path d="m15 15 6 6"/>', arrow: '<path d="M4 12h16m-6-6 6 6-6 6"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7h.01"/>', bell: '<path d="M6 8a6 6 0 0 1 12 0v7l3 3H3l3-3zM9 21h6"/>'
  };
  const icon = (name) => `<svg class="argus-icon" viewBox="0 0 24 24" aria-hidden="true">${icons[name] || icons.box}</svg>`;
  const button = (act, label, ic = '', cls = '', attrs = '') => `<button type="button" data-act="${act}" class="${cls}" ${attrs}>${ic ? icon(ic) : ''}${esc(label)}</button>`;
  const ib = (act, label, ic, attrs = '') => `<button type="button" data-act="${act}" class="argus-ghost argus-icon-button" title="${esc(label)}" aria-label="${esc(label)}" ${attrs}>${icon(ic)}</button>`;
  const signalTitle = signal => ({metrics:t('指标','Metrics'),logs:t('日志','Logs'),traces:t('链路','Traces')}[signal]);
  const language = signal => ({metrics:'PromQL',logs:'Argus KQL',traces:'GraphQL'}[signal]);
  const scopedResources = scope => scope.mode==='all_authorized'?D.resources:D.resources.filter(r=>scope.ids.includes(r.id));
  const catalogScope = () => S.catalogScope==='all'?allResources():{mode:'explicit',ids:[S.catalogScope]};
  const scopeName = scope => scope.mode==='all_authorized'?t('全部示例资源','All sample resources'):scope.ids.length===1?(D.resources.find(r=>r.id===scope.ids[0])?.name||t('资源不可用','Unavailable resource')):t(`已选 ${scope.ids.length} 项资源`,`${scope.ids.length} resources selected`);
  const scopeNames = scope => scopedResources(scope).map(r=>r.name).join(', ')||t('未选择资源','No resources selected');
  const resourceKinds = ['host','kubernetes_cluster','kubernetes_namespace','application','monitoring_target'];
  const kindName = kind => ({host:t('主机','Hosts'),kubernetes_cluster:t('集群','Clusters'),kubernetes_namespace:'Namespace',application:t('应用','Applications'),monitoring_target:t('监控目标','Monitoring targets')}[kind]);
  const metricInScope = (m,scope) => scopedResources(scope).some(r=>(r.series[m.group]||[]).length);
  const recordScope = q => scopedResources(q.executedScope).some(r=>(r.services||[]).includes(q.executedBuilder?.service));
  const selectedPanels = () => S.panels;
  const groupName = (g) => ({ all: t('全部类型', 'All types'), host: t('主机', 'Hosts'), k8s: 'Kubernetes', redis: 'Redis' }[g]);
  const units = (m, q) => m?.unit === 'ratio' ? '%' : q?.executed.includes('rate(') ? 'KiB/s' : m?.unit === 'bytes' ? 'MiB' : m?.unit || '';
  const metricFor = (q) => D.metrics.find(m => m.name === q.metric) || D.metrics.find(m => q.executed === m.query || q.executed === m.name);
  function toast(msg) { const el = $('#toast'); el.textContent = msg; el.hidden = false; clearTimeout(toastTimer); toastTimer = setTimeout(() => el.hidden = true, 3200); }
  function nav() {
    return `<aside class="argus-sidebar"><div class="argus-brand"><span class="argus-brand-mark">◎</span><div><strong>Argus</strong><small>enterprise</small></div></div>
      <div class="argus-nav-label">${t('工作空间', 'Workspace')}</div>
      <div class="argus-nav-item">${icon('grid')}${t('仪表盘', 'Dashboards')}</div><div class="argus-nav-item">${icon('box')}${t('应用', 'Applications')}</div>
      <div class="argus-nav-item">${icon('settings')}${t('监控采集', 'Collection')}</div><div class="argus-nav-item">${icon('bell')}${t('告警中心', 'Alerts')}</div>
      <button class="argus-nav-item" data-act="workspace" aria-current="page">${icon('search')}${t('观测查询','Observability queries')}<span class="argus-dot"></span></button>
      <div class="argus-nav-label">${t('资源', 'Resources')}</div>
      <button class="argus-nav-item" data-act="host-entry" title="${t('演示从主机跳入查询', 'Demo: enter from a host')}">${icon('server')}${t('主机', 'Hosts')}${icon('arrow')}</button>
      <button class="argus-nav-item" data-act="cluster-entry" title="${t('演示从集群跳入查询', 'Demo: enter from a cluster')}">${icon('box')}Kubernetes${icon('arrow')}</button>
      <div class="argus-sidebar-foot"><div class="argus-row">${icon('info')}Plan10 · ${t('交互原型', 'Prototype')}</div><p>${t('示例数据 · 未连接真实服务', 'Sample data · No live connection')}</p></div></aside>`;
  }
  function tabs() {
    return `<div class="argus-tabs" role="tablist" aria-label="${t('页面栏目', 'Page sections')}">
      ${[['query',t('查询工作台','Query workspace')],['catalog',t('指标与字段目录','Metrics & fields')]].map(([id,label]) => `<button role="tab" id="tab-${id}" aria-controls="tab-content" aria-selected="${S.tab===id}" tabindex="${S.tab===id?'0':'-1'}" data-act="tab" data-value="${id}">${esc(label)}${id==='query'?`<span class="argus-badge">${S.panels.length}</span>`:''}</button>`).join('')}</div>`;
  }
  function toolbar() {
    return `<div class="argus-toolbar"><div class="argus-row argus-wrap">      ${S.tab === 'query' ? `<label><input type="checkbox" data-control="linked" ${S.linked ? 'checked' : ''}>${t('联动十字线', 'Linked cursor')}</label>` : ''}</div>
      <div class="argus-row"><label>${icon('clock')}<select data-control="range" aria-label="${t('时间范围', 'Time range')}">${[['15', t('最近 15 分钟', 'Last 15 minutes')], ['60', t('最近 1 小时', 'Last hour')], ['360', t('最近 6 小时', 'Last 6 hours')]].map(([v,l]) => `<option value="${v}" ${S.range === v ? 'selected' : ''}>${l}</option>`).join('')}</select></label>
      ${S.tab === 'query' ? button('run-all', t('执行全部', 'Run all'), 'play', 'argus-primary') : ''}</div></div>
      <div class="argus-note">${icon('info')}<span>${t('当前仅展示示例资源；正式版按资源、来源和时间联合授权。', 'Sample resources only; production scope combines resource, source and time authorization.')}</span><a href="README.md" target="_blank">${t('查看设计说明', 'Design notes')} ↗</a></div>`;
  }
  function render() {
    const active = document.activeElement;
    const focusControl = active?.dataset?.control;
    const focusBuilder=active?.dataset?.builder, focusPicker=active?.dataset?.picker, focusPanel=active?.closest('[data-panel]')?.dataset.panel;
    const start = active?.selectionStart;
    document.documentElement.lang = S.lang === 'zh' ? 'zh-CN' : 'en';
    document.documentElement.dataset.theme = S.theme;
    $('#app').innerHTML = `${nav()}<div class="argus-shell"><header class="argus-header"><div class="argus-breadcrumb">${t('工作空间', 'Workspace')}<span>/</span><strong>${t('观测查询','Observability queries')}</strong></div><div class="argus-row"><span class="argus-badge">${t('本地原型 · 示例数据', 'Local prototype · Sample data')}</span>${ib('theme', t('切换深浅色', 'Toggle theme'), 'moon')}${button('lang', S.lang === 'zh' ? 'EN' : '中文', '', 'argus-ghost')}<span class="argus-avatar">K</span></div></header>
      <main class="argus-main"><div class="argus-heading"><div><div class="argus-row"><h1>${t('观测查询','Observability queries')}</h1><span class="argus-badge">${t('临时查询','Temporary')}</span></div><p>${t('从一个问题开始，在同一时间线上对照答案。', 'Start with a question. Compare answers on one timeline.')}</p></div><div class="argus-row">${button('add', t('新建查询', 'New query'), 'plus', 'argus-primary')}</div></div>
      ${tabs()}${toolbar()}<section id="tab-content" role="tabpanel" aria-labelledby="tab-${S.tab}">${S.tab === 'query' ? workspace() : catalog()}</section></main></div>${drawer()}${resourcePicker()}`;
    const anchor=focusPanel?`[data-panel="${focusPanel}"] `:'';
    const focusSelector=focusPicker?`[data-picker="${focusPicker}"]`:focusBuilder?`${anchor}[data-builder="${focusBuilder}"]`:focusControl?`${anchor}[data-control="${focusControl}"]`:'';
    if(focusSelector){const next=$(focusSelector);if(next){next.focus({preventScroll:true});if(typeof start==='number'&&['text','search'].includes(next.type))next.setSelectionRange(start,start);}}
    $$('.argus-shell,.argus-sidebar').forEach(el=>el.inert=Boolean(S.picker||S.drawer));
    wireCharts();
  }
  function workspace() {
    return `${selectedPanels().map((q, i) => panelView(q, i)).join('')}${button('add', t('新建查询 · 在下方添加一个独立面板', 'New query · Add an independent panel below'), 'plus', 'argus-add-panel')}<div class="argus-note" style="margin-top:var(--space-px-12)">${t('指标、日志、链路可混合排列；查询仅留在当前页面，刷新后清空。', 'Mix metrics, logs and traces in one workspace. Queries stay in memory and reset on reload.')}</div>`;
  }
  function panelView(q, i) {
    const label = String.fromCharCode(65 + i);
    const title = q.title ? tr(q.title) : q.signal === 'logs' ? t('服务日志', 'Service logs') : q.signal === 'traces' ? t('服务调用链', 'Service traces') : t('未命名查询', 'Untitled query');
    return `<article class="argus-panel" data-panel="${q.id}"><div class="argus-panel-header"><div class="argus-row"><span class="argus-panel-letter">${label}</span><span class="argus-panel-title">${esc(title)}</span><select data-control="panel-signal" aria-label="${t('查询类型','Query type')} ${label}">${['metrics','logs','traces'].map(signal=>`<option value="${signal}" ${q.signal===signal?'selected':''}>${signalTitle(signal)}</option>`).join('')}</select><span class="argus-badge">${language(q.signal)}</span>${button('panel-scope',t('资源范围 · ','Resources · ')+scopeName(q.resourceScope),'server','argus-panel-scope',`title="${esc(scopeNames(q.resourceScope))}" aria-haspopup="dialog"`)}</div><div class="argus-row">${ib('move-up', t('上移查询', 'Move query up'), 'up', i === 0 ? 'disabled' : '')}${ib('move-down', t('下移查询', 'Move query down'), 'down', i === selectedPanels().length - 1 ? 'disabled' : '')}${ib('duplicate', t('复制查询', 'Duplicate query'), 'copy')}${ib('remove', t('删除查询', 'Remove query'), 'close')}</div></div>
      <div class="argus-editor-wrap"><div class="argus-row argus-between argus-authoring"><div class="argus-segment">${button('authoring',t('构造器','Builder'),'settings','',`data-value="builder" aria-pressed="${q.authoring==='builder'}"`)}${button('authoring',t('语句','Code'),'logs','',`data-value="dsl" aria-pressed="${q.authoring==='dsl'}"`)}</div><small class="argus-tertiary">${t('仅本页临时使用','Temporary · this page only')}</small></div>${q.conversionError?`<p class="argus-conversion-error" role="alert">${t('此语句无法无损转回原型构造器，已保留完整语句。可继续在语句模式中编辑。','This expression cannot be converted losslessly by the prototype. Your full text is preserved in Code mode.')}</p>`:''}${q.authoring==='builder'?builderView(q):`<div class="argus-codebox"><span class="argus-line-no">1</span><textarea data-editor="${q.id}" aria-label="${t('查询语句', 'Query expression')} ${label}" spellcheck="false" autocomplete="off" aria-autocomplete="list" aria-expanded="false" rows="2" placeholder="${t('输入指标名或按 Ctrl + Space 查看建议…', 'Type a metric or press Ctrl + Space…')}">${esc(q.expr)}</textarea></div><div class="argus-completions" data-popup="${q.id}" hidden></div>`}
      <div class="argus-editor-help"><span>${q.authoring==='dsl'?`<span class="argus-key">Ctrl Space</span> ${t('补全','complete')} · <span class="argus-key">Shift Enter</span> ${t('执行','run')}`:t('选择条件后执行，可随时查看生成语句','Choose conditions, then run. The generated query is always available.')}</span><div class="argus-row">${q.authoring==='dsl'?button('try-complete',t('试试智能提示','Try autocomplete'),'','argus-ghost'):''}${button('advanced', t('查询选项', 'Options'), 'settings', 'argus-ghost', `aria-expanded="${q.advanced}"`)}${button(q.status === 'running' ? 'cancel' : 'run', q.status === 'running' ? t('取消', 'Cancel') : t('执行', 'Run'), q.status === 'running' ? 'close' : 'play', 'argus-primary')}</div></div></div>
      ${q.advanced ? `<div class="argus-advanced">${q.signal === 'metrics' ? `<label>${t('查询模式', 'Query mode')}<select data-control="mode"><option value="range" ${q.mode === 'range' ? 'selected' : ''}>${t('时间序列', 'Range')}</option><option value="instant" ${q.mode === 'instant' ? 'selected' : ''}>${t('即时值', 'Instant')}</option></select></label><label>${t('步长', 'Step')}<select data-control="step"><option value="auto" ${q.step === 'auto' ? 'selected' : ''}>${t('自动', 'Auto')}</option><option value="30" ${q.step === '30' ? 'selected' : ''}>30s</option><option value="60" ${q.step === '60' ? 'selected' : ''}>60s</option></select></label>` : `<span>${t('原型只执行预置示例；正式版由服务端校验语法、权限和预算。', 'This prototype runs preset fixtures; production validates syntax, authorization and budgets on the server.')}</span>`}</div>` : ''}
      <div class="argus-result-bar"><div class="argus-segment">${q.signal === 'metrics' ? `${button('graph', t('图表', 'Graph'), 'metrics', '', `aria-pressed="${q.view === 'graph'}" ${q.mode === 'instant' ? 'disabled' : ''}`)}${button('table', t('表格', 'Table'), 'grid', '', `aria-pressed="${q.view === 'table'}"`)}` : `<span class="argus-badge">${q.signal === 'logs' ? t('日志记录', 'Log records') : t('Trace 列表', 'Trace list')}</span>`}</div><span class="argus-tertiary" data-status="${q.id}"><small>${statusText(q)}</small></span></div><div data-result="${q.id}">${resultView(q)}</div></article>`;
  }
  function builderView(q) {
    const b=q.builder;
    const select=(key,label,options,disabled=false)=>`<label class="argus-builder-field"><span>${label}</span><select data-builder="${key}" ${disabled?'disabled':''}>${options.map(([v,l])=>`<option value="${esc(v)}" ${b[key]===v?'selected':''}>${esc(l)}</option>`).join('')}</select></label>`;
    const input=(key,label,type='text',attrs='')=>`<label class="argus-builder-field"><span>${label}</span><input data-builder="${key}" type="${type}" value="${esc(b[key])}" ${attrs}></label>`;
    let fields='';
    if(q.signal==='metrics') {
      const m=D.metrics.find(x=>x.name===b.metric);
      const operations=[['recipe',t('推荐计算','Recommended calculation')],['value',t('原始值','Raw value')],['avg',t('平均值','Average')],['sum',t('求和','Sum')],['min',t('最小值','Minimum')],['max',t('最大值','Maximum')]];
      if(m.type==='Counter') operations.push(['rate',t('每秒增长率 · 5m','Rate per second · 5m')]);
      fields=select('metric',t('指标','Metric'),D.metrics.map(m=>[m.name,tr(m.title)+' · '+m.name]))+
        select('operation',t('计算方式','Calculation'),operations)+
        (['avg','sum','min','max'].includes(b.operation)?select('group',t('聚合分组','Aggregate by'),[['',t('不分组','No grouping')],['argus_resource_id',t('资源','Resource')]]):'');
      fields+=`<div class="argus-builder-hint"><span class="argus-badge">${m.type} · ${m.unit}</span><small>${b.operation==='recipe'?esc(tr(m.recipe)):t('按当前条件处理示例序列','Apply the current conditions to sample series')}</small></div>`;
    } else if(q.signal==='logs') {
      fields=select('service',t('服务','Service'),[['checkout-api','checkout-api'],['inventory-api','inventory-api']])+
        select('level',t('日志级别','Log level'),[['',t('全部级别','All levels')],['ERROR','ERROR'],['WARN','WARN'],['INFO','INFO']])+
        input('keyword',t('正文包含','Message contains'))+
        select('output',t('返回内容','Return'),[['records',t('日志记录','Log records')],['count',t('记录数量','Record count')]])+
        select('limit',t('记录上限','Record limit'),[['1','1'],['20','20'],['100','100']],b.output==='count');
    } else {
      fields=select('service',t('入口服务','Root service'),[['checkout-api','checkout-api'],['inventory-api','inventory-api']])+
        select('status',t('调用状态','Trace status'),[['',t('全部状态','All statuses')],['ERROR','ERROR'],['OK','OK']])+
        input('duration',t('最短耗时（ms）','Minimum duration (ms)'),'number','min="0" max="3600000"')+
        input('trace','Trace ID')+
        select('limit',t('记录上限','Record limit'),[['1','1'],['20','20'],['100','100']]);
    }
    return `<div class="argus-builder-grid ${q.signal==='metrics'?'argus-builder-metrics':''}">${fields}</div><details class="argus-generated"><summary>${t('查看生成语句','View generated query')} · ${language(q.signal)}</summary><pre>${esc(q.expr)}</pre></details>`;
  }
  function statusText(q) {
    const labels = { running: t('正在读取示例…', 'Loading fixture…'), cancelled: t('已取消', 'Cancelled'), idle: t('尚未执行', 'Not run'), dirty: t('条件已修改 · 请重新执行', 'Changed · Run again'), empty: t('当前范围没有样本', 'No samples in scope'), error: t('原型未执行此表达式', 'Expression not evaluated'), ready: t('示例结果 · 2026-10-27 11:00 CST', 'Fixture · 2026-10-27 11:00 CST') };
    return labels[q.status];
  }
  function resultView(q) {
    if (['idle', 'running', 'dirty', 'cancelled'].includes(q.status)) return `<div class="argus-empty">${q.status === 'running' ? t('正在加载示例结果…', 'Loading fixture results…') : t('执行查询后在这里查看结果', 'Run the query to view results here')}</div>`;
    if (q.status === 'error') return `<div class="argus-empty"><strong class="argus-error">${t('此原型不执行任意查询语句', 'This prototype does not evaluate arbitrary queries')}</strong>${t('输入已保留。请从目录加载预置示例；正式版将使用现有查询引擎。', 'Your text is preserved. Load a preset from the catalog; production uses the existing query engine.')}${button('tab', t('打开目录', 'Open catalog'), 'book', '', 'data-value="catalog"')}</div>`;
    if (q.status === 'empty' && q.signal !== 'metrics') return `<div class="argus-empty">${t('当前筛选没有示例记录','No fixture records match these filters')}</div>`;
    if (q.signal !== 'metrics') {
      if (!recordScope(q)) return `<div class="argus-empty"><strong>${t('当前资源范围没有示例记录', 'No fixture records in this resource scope')}</strong>${t('日志与链路样例属于 dev-cluster；切换到该集群或全部示例资源查看。', 'Log and trace fixtures belong to dev-cluster. Select that cluster or all sample resources.')}</div>`;
      return q.signal === 'logs' ? logResults(q) : traceResults(q);
    }
    const m = metricFor(q);
    if (!m || !m.observed || !metricInScope(m,q.executedScope) || q.status === 'empty') return `<div class="argus-empty"><strong>${t('当前范围未观测到该指标', 'Metric not observed in this scope')}</strong>${t('调整资源或时间范围，或在指标目录检查采集来源。无数据不表示数值为 0。', 'Adjust scope or time, or inspect the collection source in the catalog. Missing data is not zero.')}</div>`;
    return q.view === 'table' || q.mode === 'instant' ? metricTable(q, m) : chart(q, m);
  }
  function baseNames(m,q) {return [...new Set(scopedResources(q.executedScope).flatMap(r=>r.series[m.group]||[]))];}
  function fixtureValues(m, line = 0, count = 61) {
    return Array.from({ length: count }, (_, j) => { const i = j * 60 / (count - 1); const wave = Math.sin(i * .29 + line) * 3 + Math.sin(i * 1.4) * 1.2; const peak = Math.max(0, 1 - Math.abs(i - 39) / 6) * 21; const mem = m.name.includes('memory'); const ratio = m.unit === 'ratio'; return ratio ? (mem ? 51 + line * 9 + wave * .35 + peak * .22 : 24 + line * 10 + wave + peak) : m.unit === 'cores' ? (25 + wave + peak) / 100 : (mem ? 340 + line * 100 + wave * 3 + peak * 2 : 100 + line * 60 + wave * 4 + peak * 7); });
  }
  function names(m,q) {return q?.series?q.series.map(x=>x.name):baseNames(m,q);}
  function values(m,line=0,count=61,q) {
    const arr=q?.series?.[line]?.values;
    if(!arr)return fixtureValues(m,line,count);
    return Array.from({length:count},(_,i)=>{const x=i*60/(count-1),lo=Math.floor(x),hi=Math.min(60,lo+1);return arr[lo]+(arr[hi]-arr[lo])*(x-lo);});
  }
  function ticks() { return Array.from({ length: 7 }, (_, i) => { const min = 660 - Number(S.range) + Number(S.range) * i / 6; return `${String(Math.floor(min / 60)).padStart(2, '0')}:${String(Math.floor(min % 60)).padStart(2, '0')}`; }); }
  function chart(q, m) {
    const lines=names(m,q), peak=Math.max(...lines.flatMap((_,i)=>values(m,i,61,q)));const max=m.unit==='ratio'?Math.max(100,Math.ceil(peak/100)*100):m.unit==='cores'?Math.max(1,Math.ceil(peak)):Math.ceil(peak/100)*100;
    const count = q.step === 'auto' ? 61 : Math.min(721, Math.floor(Number(S.range) * 60 / Number(q.step)) + 1);
    const paths = lines.map((_, k) => values(m, k, count,q).map((v, i) => `${i ? 'L' : 'M'}${48 + i * 940 / (count - 1)},${130 - v / max * 116}`).join(' '));
    return `<div class="argus-chart-wrap" data-chart="${q.id}"><svg class="argus-chart" viewBox="0 0 1010 166" preserveAspectRatio="none" role="img" aria-label="${esc(tr(m.title))} ${t('示例趋势图', 'sample chart')}">
      ${[0,1,2,3,4].map(i => `<line class="argus-grid-line" x1="48" x2="988" y1="${14 + i * 29}" y2="${14 + i * 29}"/><text x="36" y="${18 + i * 29}" text-anchor="end">${max * (4 - i) / 4}</text>`).join('')}
      <path class="argus-chart-area" d="${paths[0]} L988,130 L48,130 Z"/>${paths.map((p,k) => `<path class="argus-chart-line ${k ? 'argus-second' : ''}" d="${p}"/>`).join('')}
      ${ticks().map((s,i) => `<text x="${48+i*940/6}" y="156" text-anchor="middle">${s}</text>`).join('')}<line class="argus-crosshair" x1="500" x2="500" y1="14" y2="130"/>
      <rect class="argus-hover-capture" x="48" y="14" width="940" height="116" fill="transparent"/></svg></div>
      <div class="argus-legend"><div class="argus-row">${lines.map((n,i) => `<span><i class="argus-legend-mark ${i ? 'argus-second' : ''}"></i>${esc(n)}</span>`).join('')}</div><span class="argus-hover-readout" data-readout="${q.id}">${t('单位', 'Unit')}: ${units(m,q)} · ${count} ${t('示例点 / 序列', 'fixture points / series')}</span></div>`;
  }
  function metricTable(q,m) {
    return `<div class="argus-chart-wrap"><table><thead><tr><th>${t('资源 / 序列', 'Resource / series')}</th><th>${t('取值时间', 'Timestamp')}</th><th>${t('最后值', 'Last value')}</th><th>${t('数据来源', 'Source')}</th></tr></thead><tbody>${names(m,q).map((n,i) => `<tr><td><code>${esc(n)}</code></td><td>2026-10-27 11:00 CST</td><td>${values(m,i,61,q).at(-1).toFixed(2)} ${units(m,q)}</td><td>${m.source}</td></tr>`).join('')}</tbody></table><small class="argus-tertiary">${t('示例结果；表格取范围最后一个点，即时模式取统一结束时间。', 'Fixture results. Range tables show the last point; instant mode uses the shared end time.')}</small></div>`;
  }
  function logResults(q) {
    if(q.executedBuilder.output==='count') return `<div class="argus-chart-wrap"><table><thead><tr><th>${t('匹配记录数','Matching records')}</th><th>${t('口径','Scope')}</th></tr></thead><tbody><tr><td>${(q.rows||filteredRecords(q)).length}</td><td>${t('当前范围与条件下的示例记录','Fixture records matching the current scope and conditions')}</td></tr></tbody></table></div>`;
    return `<div class="argus-chart-wrap"><table><thead><tr><th>${t('时间', 'Time')}</th><th>${t('级别', 'Level')}</th><th>${t('服务', 'Service')}</th><th>${t('日志内容', 'Message')}</th><th></th></tr></thead><tbody>${(q.rows||filteredRecords(q)).slice(0,Number(q.executedBuilder.limit)).map(l => {const i=D.logs.indexOf(l);return `<tr><td><code>${l.time}</code></td><td><span class="argus-badge ${l.level === 'ERROR' ? 'argus-warning' : ''}">${l.level}</span></td><td>${l.service}</td><td>${esc(l.body)}</td><td>${button('expand-log', t('详情', 'Details'), '', 'argus-ghost', `data-value="${i}"`)}</td></tr>${q.expanded === i ? `<tr class="argus-detail-row"><td colspan="5"><pre>${esc(JSON.stringify({ 'service_name': l.service, 'severity_text': l.level, 'trace_id': l.trace, body: l.body, sample: true },null,2))}</pre>${button('related-trace', t('查看关联链路', 'View related trace'), 'traces', '', `data-value="${i}"`)}</td></tr>` : ''}`;}).join('')}</tbody></table></div>`;
  }
  function traceResults(q) {
    const filtered = (q.rows||filteredRecords(q)).slice(0,Number(q.executedBuilder.limit));
    return `<div class="argus-chart-wrap"><table><thead><tr><th>Trace ID</th><th>${t('入口服务', 'Root service')}</th><th>${t('持续时间', 'Duration')}</th><th>${t('状态', 'Status')}</th><th></th></tr></thead><tbody>${filtered.map(l => {const i = D.logs.indexOf(l); return `<tr><td><code>${l.trace}</code></td><td>${l.service}</td><td>${[3210,182,46][i]} ms</td><td><span class="argus-badge ${i === 0 ? 'argus-warning' : 'argus-success'}">${i === 0 ? 'ERROR' : 'OK'}</span></td><td>${button('trace-detail', t('调用详情', 'Trace details'), 'arrow', 'argus-ghost', `data-value="${i}"`)}</td></tr>`;}).join('')}</tbody></table></div>`;
  }
  function catalog() {
    const switcher = `<div class="argus-row argus-catalog-signals">${['metrics','logs','traces'].map(signal=>button('catalog-signal',signalTitle(signal),signal,S.catalogSignal===signal?'argus-primary':'',`data-value="${signal}" aria-pressed="${S.catalogSignal===signal}"`)).join('')}</div>`;
    if (S.catalogSignal !== 'metrics') return switcher + signalCatalog();
    const rows = D.metrics.filter(m => (S.group === 'all' || m.group === S.group) && metricInScope(m,catalogScope()) && (!S.observed || m.observed) && `${m.name} ${m.title.join(' ')}`.toLowerCase().includes(S.search.toLowerCase()));
    return switcher + `<div class="argus-catalog-summary">${icon('book')}<div><strong>${t('先找到指标，再选择合适的查询方式', 'Find the metric, then choose the right query')}</strong><div><small>${t('这里同时回答：采到了什么、来自哪里、是什么意思，以及应该怎么查。', 'See what was observed, where it comes from, what it means and how to query it.')}</small></div></div></div>
      <div class="argus-filter"><input type="search" data-control="search" aria-label="${t('搜索指标', 'Search metrics')}" placeholder="${t('搜索指标名称或含义…', 'Search metric name or meaning…')}" value="${esc(S.search)}"><label>${t('资源范围','Resource scope')}<select data-control="catalog-scope" aria-label="${t('目录资源范围','Catalog resource scope')}"><option value="all" ${S.catalogScope==='all'?'selected':''}>${t('全部示例资源','All sample resources')}</option>${D.resources.map(r=>`<option value="${r.id}" ${S.catalogScope===r.id?'selected':''}>${esc(r.name)}</option>`).join('')}</select></label><select data-control="group" aria-label="${t('资源类型', 'Resource type')}">${['all','host','k8s','redis'].map(g => `<option value="${g}" ${S.group === g ? 'selected' : ''}>${groupName(g)}</option>`).join('')}</select><label><input type="checkbox" data-control="observed" ${S.observed ? 'checked' : ''}>${t('仅看已观测指标', 'Observed metrics only')}</label></div>
      <div class="argus-table-wrap"><table><thead><tr><th>${t('指标名称 / 含义', 'Metric / meaning')}</th><th>${t('类型', 'Type')}</th><th>${t('原始单位', 'Raw unit')}</th><th>${t('资源 / 采集来源', 'Resource / source')}</th><th>${t('观测状态', 'Observation')}</th><th>${t('操作', 'Action')}</th></tr></thead><tbody>${rows.map(m => `<tr><td>${button('metric-detail', m.name, '', 'argus-metric-link', `data-value="${m.name}"`)}<small>${esc(tr(m.title))}</small></td><td><span class="argus-badge">${m.type}</span></td><td>${m.unit === 'ratio' ? '0–1' : m.unit}</td><td>${groupName(m.group)}<small>${m.source}</small></td><td><span class="argus-badge ${m.observed ? 'argus-success' : 'argus-warning'}"><i class="argus-dot"></i>${m.observed ? t('示例已观测', 'Sample observed') : t('示例未观测', 'Sample unobserved')}</span></td><td>${button('use-metric', t('新建查询', 'New query'), 'plus', 'argus-ghost', `data-value="${m.name}"`)}</td></tr>`).join('') || `<tr><td colspan="6" class="argus-empty">${t('没有匹配的指标，请调整筛选。', 'No matching metrics. Adjust the filters.')}</td></tr>`}</tbody></table></div><div class="argus-table-footer"><span>${rows.length} ${t('项示例指标', 'sample metrics')}</span><span>${t('正式版：服务端搜索与分页 · 按当前授权范围发现', 'Production: server search and pagination within current authorization')}</span></div>`;
  }
  function signalCatalog() {
    const isLogs = S.catalogSignal === 'logs', fields = isLogs ? D.logFields : D.traceFields;
    return `<div class="argus-catalog-summary">${icon('book')}${isLogs ? t('日志使用 Argus KQL；字段和候选值来自当前范围的实际日志。', 'Logs use Argus KQL; fields and values come from records in scope.') : t('链路使用只读 GraphQL；字段补全来自 Argus 固定 Schema。', 'Traces use read-only GraphQL; field completions come from the Argus schema.')}</div><div class="argus-table-wrap"><table><thead><tr><th>${t('字段 / 方法', 'Field / operation')}</th><th>${t('来源', 'Source')}</th></tr></thead><tbody>${fields.map(f => `<tr><td><code>${f}</code></td><td>${isLogs ? t('日志字段目录（示例）', 'Log field catalog (fixture)') : t('Argus 只读 SDL', 'Argus read-only SDL')}</td></tr>`).join('')}</tbody></table></div><div class="argus-recipe" style="margin-top:var(--space-px-16)"><h3>${t('按服务查询', 'Query by service')}</h3><pre>${esc(isLogs ? B.compile('logs',B.defaults('logs')) : D.traceQuery)}</pre>${button('use-signal-example', t('新建查询', 'New query'), 'plus', 'argus-primary')}</div>`;
  }

  function drawer() {
    if (!S.drawer) return '';
    const x = S.drawer;
    let content = '';
    if (x.type === 'metric') {
      const m = D.metrics.find(m => m.name === x.name);
      content = `<h2 id="drawer-title">${esc(tr(m.title))}</h2><p><code>${m.name}</code></p><p>${esc(tr(m.desc))}</p><div class="argus-definition"><div><small>${t('类型', 'Type')}</small>${m.type}</div><div><small>${t('原始单位', 'Raw unit')}</small>${m.unit}</div><div><small>${t('采集来源', 'Source')}</small>${m.source}</div><div><small>${t('当前范围', 'Current scope')}</small>${esc(scopeName(catalogScope()))}</div></div>
        <div class="argus-drawer-section"><h3>${t('可用标签（示例）', 'Available labels (fixture)')}</h3><div class="argus-row argus-wrap">${m.labels.map(l => `<code class="argus-badge">${l}</code>`).join('')}</div></div><div class="argus-drawer-section"><h3>${t('推荐语句', 'Recommended expression')}</h3><div class="argus-recipe"><strong>${esc(tr(m.recipe))}</strong><pre>${esc(m.query)}</pre><p><small>${esc(tr(m.caution))}</small></p><div class="argus-drawer-actions">${button('copy-expression', t('复制语句', 'Copy expression'), 'copy', '', `data-value="${m.name}"`)}${button('use-metric', t('新建查询', 'New query'), 'plus', 'argus-primary', `data-value="${m.name}"`)}</div></div></div>
        <div class="argus-drawer-section"><h3>${t('采集与观测', 'Collection & observation')}</h3><p>${m.observed ? t('此原型有该指标的示例样本。正式版需展示实际最后采样时间、覆盖资源数和目录完整性。', 'This prototype has fixture samples. Production must show actual last-sample time, resource coverage and catalog completeness.') : t('集成定义支持该指标，但本例没有样本。新建查询将返回无数据。', 'The integration defines this metric, but this fixture has no samples. A new query returns no data.')}</p></div>`;
    } else {
      const l = D.logs[x.index];
      content = `<h2 id="drawer-title">${t('调用详情 · 示例', 'Trace details · Fixture')}</h2><p><code>${l.trace}</code></p><span class="argus-badge argus-warning">${t('仅表示已接收样本，不证明全链路完整', 'Received spans only; completeness is not proven')}</span><div class="argus-drawer-section"><table><thead><tr><th>Span</th><th>${t('持续时间', 'Duration')}</th></tr></thead><tbody>${[['checkout-api', 100], ['inventory-api', 62], ['redis GET', 88]].map(([n,w]) => `<tr><td>${n}<div><span class="argus-waterfall" style="width:${w}%"></span></div></td><td>${Math.round([3210,182,46][x.index] * w / 100)} ms</td></tr>`).join('')}</tbody></table></div>`;
    }
    return `<div class="argus-drawer-backdrop" data-act="close-drawer"></div><aside class="argus-drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title"><div class="argus-row argus-between"><span class="argus-muted">${t('详情', 'Details')}</span>${ib('close-drawer', t('关闭详情', 'Close details'), 'close')}</div>${content}</aside>`;
  }
  function resourcePicker() {
    const p=S.picker;if(!p)return '';
    const q=S.panels.find(q=>q.id===p.panelId);if(!q)return '';
    const title=q.title?tr(q.title):signalTitle(q.signal);
    const rows=D.resources.filter(r=>r.kind===p.kind&&`${r.name} ${r.parent}`.toLowerCase().includes(p.search.toLowerCase()));
    const summary=scopeName(p);
    return `<div class="argus-drawer-backdrop" data-act="scope-cancel"></div><section class="argus-resource-modal" role="dialog" aria-modal="true" aria-labelledby="scope-dialog-title" aria-describedby="scope-dialog-help">
      <header class="argus-resource-modal-heading"><div><h2 id="scope-dialog-title">${t('选择资源范围','Select resource scope')}</h2><p id="scope-dialog-help">${t('仅应用于查询','Applies only to query')} ${String.fromCharCode(65+S.panels.indexOf(q))} · ${esc(title)}</p></div>${ib('scope-cancel',t('关闭资源选择','Close resource picker'),'close')}</header>
      <div class="argus-resource-modal-body"><div class="argus-tabs" role="tablist" aria-label="${t('资源类型','Resource types')}">${resourceKinds.map(kind=>`<button type="button" role="tab" data-act="scope-kind" data-value="${kind}" aria-selected="${p.kind===kind}" tabindex="${p.kind===kind?'0':'-1'}" aria-controls="scope-resource-list">${kindName(kind)}</button>`).join('')}</div>
      <div class="argus-row argus-wrap argus-scope-actions">${button('scope-all',t('全部有权资源（示例）','All authorized resources (sample)'),'','',`aria-pressed="${p.mode==='all_authorized'}"`)}${button('scope-clear',t('清空选择','Clear selection'))}<span role="status" class="argus-muted">${esc(summary)}</span></div>
      <input class="argus-scope-search" type="search" data-picker="search" aria-label="${t('搜索资源','Search resources')}" placeholder="${t('搜索名称或所属资源','Search name or parent resource')}" value="${esc(p.search)}">
      <div id="scope-resource-list" role="tabpanel" class="argus-table-wrap"><table><thead><tr><th>${t('名称','Name')}</th><th>${t('环境','Environment')}</th><th>${t('所属资源','Parent resource')}</th><th>${t('选择','Selection')}</th></tr></thead><tbody>${rows.map(r=>`<tr><td>${esc(r.name)}</td><td>${t('测试','Test')}</td><td>${esc(r.parent)}</td><td>${button('scope-toggle',p.mode==='explicit'&&p.ids.includes(r.id)?t('已选择','Selected'):t('选择','Select'),p.mode==='explicit'&&p.ids.includes(r.id)?'check':'plus','',`data-value="${r.id}" aria-label="${t('选择','Select')} ${esc(r.name)}" aria-pressed="${p.mode==='explicit'&&p.ids.includes(r.id)}"`)}</td></tr>`).join('')||`<tr><td colspan="4" class="argus-empty">${t('没有匹配的资源','No matching resources')}</td></tr>`}</tbody></table></div>
      <div class="argus-scope-selected"><small class="argus-muted">${t('已选资源','Selected resources')}</small><div class="argus-row argus-wrap">${p.mode==='all_authorized'?`<span class="argus-badge">${t('全部有权资源（示例）','All authorized resources (sample)')}</span>`:p.ids.length?p.ids.map(id=>{const r=D.resources.find(r=>r.id===id);return button('scope-toggle',r.name,'close','argus-scope-chip',`data-value="${id}" aria-label="${t('移除','Remove')} ${esc(r.name)}"`);}).join(''):`<span class="argus-tertiary">${t('未选择资源，查询将返回无数据','No resources selected; the query will return no data')}</span>`}</div></div></div>
      <footer class="argus-resource-modal-footer"><small>${t('搜索和切换类型保留已选项；确认前不修改查询。','Search and type changes retain selection. The query changes only after confirmation.')}</small><div class="argus-row">${button('scope-cancel',t('取消','Cancel'))}${button('scope-confirm',t('确认选择','Confirm selection'),'check','argus-primary')}</div></footer></section>`;
  }
  function openResourcePicker(q) {
    S.drawer=null;S.picker={panelId:q.id,...structuredClone(q.resourceScope),kind:D.resources.find(r=>q.resourceScope.ids.includes(r.id))?.kind||'host',search:''};
    closeCompletion();render();$('[data-picker="search"]')?.focus();
  }
  function closeResourcePicker() {const id=S.picker?.panelId;S.picker=null;render();$(`[data-panel="${id}"] [data-act="panel-scope"]`)?.focus({preventScroll:true});}
  function confirmResourceScope() {
    const p=S.picker,q=S.panels.find(q=>q.id===p.panelId);if(!q){closeResourcePicker();return;}
    const next={mode:p.mode,ids:p.mode==='explicit'?[...p.ids].sort():[]};
    if(q.resourceScope.mode!==next.mode||[...q.resourceScope.ids].sort().join('|')!==next.ids.join('|')){q.resourceScope=next;markDirty(q);}
    closeResourcePicker();
  }
  function openDrawer(value) { lastDrawerFocus = document.activeElement; S.drawer = value; render(); setTimeout(() => $('.argus-drawer input, .argus-drawer button')?.focus(), 0); }
  function closeDrawer() { S.drawer = null; render(); const control = lastDrawerFocus?.dataset; if (control?.act) $(`[data-act="${control.act}"]${control.value ? `[data-value="${control.value}"]` : ''}`)?.focus(); }
  function invalidate() { S.panels.forEach(q => { q.revision++; q.status = q.expr ? 'dirty' : 'idle'; }); closeCompletion(); }
  function addMetric(name) { const m = D.metrics.find(x => x.name === name); S.tab = 'query'; S.drawer = null; const q = panel('metrics',m); q.resourceScope=structuredClone(catalogScope()); q.status = 'dirty'; selectedPanels().push(q); render(); focusPanel(q); run(q); }
  function focusPanel(q) { setTimeout(() => { const el = $(`[data-panel="${q.id}"] textarea, [data-panel="${q.id}"] [data-builder]`); el?.scrollIntoView({block: 'center', behavior: 'smooth'}); el?.focus({preventScroll:true}); }, 0); }
  function markDirty(q) { q.revision++; q.status='dirty'; q.conversionError=false; }
  function filteredRecords(q) {
    const b=q.executedBuilder;
    if(!b || !recordScope(q)) return [];
    return D.logs.filter((l,i)=>l.service===b.service && (q.signal==='logs' ?
      (!b.level||l.level===b.level)&&(!b.keyword||l.body.toLowerCase().includes(b.keyword.toLowerCase())) :
      (!b.status||b.status===(i===0?'ERROR':'OK'))&&(!b.duration||[3210,182,46][i]>=Number(b.duration))&&(!b.trace||l.trace===b.trace)));
  }
  function metricSeries(q,m) {
    const b=q.executedBuilder;
    let items=baseNames(m,q).map(name=>({name,values:fixtureValues(m,[...new Set(D.resources.flatMap(r=>r.series[m.group]||[]))].indexOf(name))}));
    if(m.name==='system_cpu_utilization'&&b.operation!=='recipe') items=items.map(x=>({...x,values:x.values.map(v=>100-v)}));
    if(m.type==='Counter'&&!['recipe','rate'].includes(b.operation)) items=items.map(x=>({...x,values:x.values.map((_,i,vs)=>100+vs.slice(0,i+1).reduce((a,b)=>a+b,0)*Number(S.range)/60/1024)}));
    if(['avg','sum','min','max'].includes(b.operation)&&!b.group&&items.length) {
      const values=items[0].values.map((_,i)=>{const vs=items.map(x=>x.values[i]);return b.operation==='avg'?vs.reduce((a,b)=>a+b,0)/vs.length:b.operation==='sum'?vs.reduce((a,b)=>a+b,0):Math[b.operation](...vs);});
      items=[{name:b.operation,values}];
    }
    return items;
  }
  function validateBuilder(q) {const invalid=$(`[data-panel="${q.id}"] input:invalid`);if(invalid){invalid.reportValidity();return false;}return true;}
  function run(q) {
    if(!validateBuilder(q))return;
    closeCompletion();q.status='running';const rev=++q.revision;q.executed=q.expr.trim();q.executedScope=structuredClone(q.resourceScope);q.executedBuilder=B.parse(q.signal,q.executed);render();
    setTimeout(()=>{
      if(rev!==q.revision||!S.panels.includes(q))return;
      const b=q.executedBuilder;
      if(!b)q.status='error';
      else if(q.signal==='metrics'){
        const m=D.metrics.find(x=>x.name===b.metric);q.metric=m.name;q.title=m.title;q.series=metricSeries(q,m);
        q.status=!m.observed||!metricInScope(m,q.executedScope)||!q.series.length?'empty':'ready';
      }else{q.rows=filteredRecords(q);q.status=!recordScope(q)||(!q.rows.length&&!(q.signal==='logs'&&b.output==='count'))?'empty':'ready';}
      render();
    },420);
  }
  function switchAuthoring(q,mode){
    closeCompletion();if(mode===q.authoring||!validateBuilder(q))return;
    if(mode==='builder'){const parsed=B.parse(q.signal,q.expr);if(!parsed){q.conversionError=true;render();return;}q.builder=parsed;}else q.builder=null;
    q.authoring=mode;q.conversionError=false;render();
  }
  function switchSignal(q,signal){
    if(signal===q.signal)return;
    const {drafts,id,revision,resourceScope,...old}=q;drafts[q.signal]=structuredClone(old);
    const next=drafts[signal]||panel(signal);
    Object.assign(q,structuredClone(next),{id,signal,drafts,resourceScope,revision:revision+1,status:'dirty',conversionError:false});closeCompletion();render();
  }
  function addSignal(signal){const q=panel(signal);S.tab='query';S.panels.push(q);render();focusPanel(q);return q;}

  document.addEventListener('click',async e=>{
    const target=e.target.closest('[data-act]');if(!target||target.disabled)return;
    const act=target.dataset.act,val=target.dataset.value;
    const q=S.panels.find(p=>p.id===Number(target.closest('[data-panel]')?.dataset.panel)),index=S.panels.indexOf(q);
    if(act==='panel-scope')openResourcePicker(q);
    else if(act==='scope-cancel')closeResourcePicker();
    else if(act==='scope-confirm')confirmResourceScope();
    else if(act==='scope-kind'){S.picker.kind=val;S.picker.search='';render();}
    else if(act==='scope-all'){S.picker.mode='all_authorized';S.picker.ids=[];render();}
    else if(act==='scope-clear'){S.picker.mode='explicit';S.picker.ids=[];render();}
    else if(act==='scope-toggle'){const p=S.picker;if(p.mode!=='explicit'){p.mode='explicit';p.ids=[];}p.ids=p.ids.includes(val)?p.ids.filter(x=>x!==val):[...p.ids,val];render();}
    else if(act==='workspace'){S.tab='query';render();}
    else if(act==='catalog-signal'){S.catalogSignal=val;render();}
    else if(act==='tab'){S.tab=val;closeCompletion();render();}
    else if(act==='theme'){S.theme=S.theme==='light'?'dark':'light';render();}
    else if(act==='lang'){S.lang=S.lang==='zh'?'en':'zh';render();}
    else if(act==='add')addSignal('metrics');
    else if(act==='authoring')switchAuthoring(q,val);
    else if(act==='run')run(q);
    else if(act==='run-all')S.panels.forEach(run);
    else if(act==='cancel'){q.revision++;q.status='cancelled';render();}
    else if(act==='remove'){q.revision++;S.panels.splice(index,1);render();}
    else if(act==='duplicate'){const next={...structuredClone(q),id:++serial,revision:0,status:'dirty'};S.panels.splice(index+1,0,next);render();focusPanel(next);}
    else if(act==='move-up'||act==='move-down'){const to=index+(act==='move-up'?-1:1);[S.panels[index],S.panels[to]]=[S.panels[to],S.panels[index]];render();}
    else if(act==='graph'||act==='table'){q.view=act;render();}
    else if(act==='advanced'){q.advanced=!q.advanced;render();}
    else if(act==='try-complete'){q.expr=q.signal==='metrics'?'system_':q.signal==='logs'?'service_':'{ query';markDirty(q);render();const el=$(`[data-editor="${q.id}"]`);el.focus();el.setSelectionRange(el.value.length,el.value.length);showCompletion(el,true);}
    else if(act==='complete')applyCompletion(Number(val));
    else if(act==='metric-detail')openDrawer({type:'metric',name:val});
    else if(act==='use-metric')addMetric(val);
    else if(act==='copy-expression'){try{await navigator.clipboard.writeText(D.metrics.find(m=>m.name===val).query);toast(t('语句已复制','Expression copied'));}catch{toast(t('请选中上方语句后复制','Select and copy the expression above'));}}
    else if(act==='close-drawer')closeDrawer();
    else if(act==='expand-log'){q.expanded=q.expanded===Number(val)?-1:Number(val);render();}
    else if(act==='trace-detail')openDrawer({type:'trace',index:Number(val)});
    else if(act==='related-trace'){const next=addSignal('traces');next.resourceScope=structuredClone(q.resourceScope);next.builder.trace=D.logs[Number(val)].trace;next.expr=B.compile('traces',next.builder);render();run(next);}
    else if(act==='use-signal-example')addSignal(S.catalogSignal);
    else if(act==='host-entry'||act==='cluster-entry'){const next=panel('metrics',D.metrics[act==='host-entry'?0:3]);next.resourceScope={mode:'explicit',ids:[act==='host-entry'?'demo-host-b04':'demo-cluster']};S.panels.push(next);S.tab='query';render();focusPanel(next);toast(t('已新建该资源的临时查询','Created a temporary query for this resource'));}
  });
  document.addEventListener('change',e=>{
    const q=S.panels.find(p=>p.id===Number(e.target.closest('[data-panel]')?.dataset.panel)),key=e.target.dataset.builder;
    if(key&&q&&e.target.tagName==='INPUT')return;
    if(key&&q){q.builder[key]=e.target.value;if(key==='metric'){q.builder=B.defaults('metrics',e.target.value);q.title=D.metrics.find(m=>m.name===e.target.value).title;}if(key==='operation')q.builder.group='';q.expr=B.compile(q.signal,q.builder);markDirty(q);render();return;}
    const ctl=e.target.dataset.control;if(!ctl)return;const val=e.target.value;
    if(ctl==='panel-signal'){switchSignal(q,val);return;}
    if(ctl==='range'){invalidate();S.range=val;}
    else if(ctl==='catalog-scope')S.catalogScope=val;
    else if(ctl==='group')S.group=val;
    else if(ctl==='observed'||ctl==='linked')S[ctl]=e.target.checked;
    else if(ctl==='mode'||ctl==='step'){q[ctl]=val;markDirty(q);if(ctl==='mode'&&val==='instant')q.view='table';}
    render();
  });

  document.addEventListener('input', e => {
    if(e.target.dataset.picker==='search'&&S.picker){S.picker.search=e.target.value;render();return;}
    if(e.target.dataset.builder){
      const q=S.panels.find(p=>p.id===Number(e.target.closest('[data-panel]').dataset.panel));q.builder[e.target.dataset.builder]=e.target.value;q.expr=B.compile(q.signal,q.builder);markDirty(q);
      $(`[data-panel="${q.id}"] .argus-generated pre`).textContent=q.expr;$(`[data-status="${q.id}"]`).textContent=statusText(q);$(`[data-result="${q.id}"]`).innerHTML=resultView(q);return;
    }
    if (e.target.dataset.control === 'search') { S.search = e.target.value; render(); return; }
    const id = e.target.dataset.editor; if (!id) return;
    const q = selectedPanels().find(p => p.id === Number(id)); q.expr = e.target.value; markDirty(q);
    $(`[data-status="${id}"]`).textContent = statusText(q); $(`[data-result="${id}"]`).innerHTML = resultView(q);
    showCompletion(e.target);
  });
  function completionContext(el) {
    const currentPanel=S.panels.find(p=>p.id===Number(el.dataset.editor));
    const signal=currentPanel.signal;
    const before = el.value.slice(0,el.selectionStart), after = el.value.slice(el.selectionEnd);
    let prefix = (before.match(/[a-zA-Z_][a-zA-Z0-9_.]*$/) || [''])[0], items = [], kind = t('指标与函数', 'Metrics & functions');
    if (signal !== 'metrics') {
      items = (signal === 'logs' ? D.logFields : D.traceFields).map(n => ({label:n, insert:n, desc:t('当前语言字段', 'Field in this language'), tag:language(signal)}));
    } else {
      const matchMetric = [...before.matchAll(/([a-zA-Z_:][a-zA-Z0-9_:]*)\s*\{/g)].at(-1)?.[1];
      const m = D.metrics.find(m => m.name === matchMetric);
      const unmatched = before.lastIndexOf('{') > before.lastIndexOf('}');
      const value = unmatched && before.match(/([a-zA-Z_][a-zA-Z0-9_]*)\s*(?:=~|!~|!=|=)\s*"([^"\n]*)$/);
      if (value) {
        prefix = value[2]; kind = t('标签值 · 当前范围示例', 'Label values · Scoped fixtures');
        const map = {state:['idle','used','free','user','system'], direction:['receive','transmit'], device:['eth0','lo'], cpu:['0','1'], argus_resource_id:scopedResources(currentPanel.resourceScope).map(r=>r.id), k8s_namespace_name:['default','checkout'], container_name:['checkout-api','inventory-api']};
        items = (map[value[1]] || []).map(n => ({label:n, insert:n, desc:t('选择值；权限范围仍由服务端约束', 'Select a value; the server still enforces scope'), tag:'value'}));
      } else if (unmatched) {
        kind = t('标签名', 'Label names');
        items = (m?.labels || []).map(n => ({label:n, insert:`${n}=""`, desc:t('来自该指标的标签目录', 'From this metric’s label catalog'), tag:'label', cursorBack:1}));
      } else if (/\[[0-9a-z]*$/.test(before)) {
        prefix = (before.match(/\[([0-9a-z]*)$/) || [,''])[1]; kind = t('范围窗口', 'Range duration');
        items = ['1m','5m','15m','1h'].map(n => ({label:n, insert:n, desc:t('窗口应覆盖多个采样点', 'Use a window covering several samples'), tag:'duration'}));
      } else {
        items = D.metrics.filter(m => m.observed && metricInScope(m,currentPanel.resourceScope)).map(m => ({label:m.name, insert:m.name, desc:tr(m.title), tag:m.type})).concat(D.functions.map(f => ({label:f.name, insert:f.insert, desc:tr(f.desc), tag:'function'})));
      }
    }
    const suffixToken = (after.match(/^[a-zA-Z0-9_.]*/) || [''])[0];
    return {items:items.filter(x => x.label.toLowerCase().startsWith(prefix.toLowerCase())).slice(0,7), start:el.selectionStart-prefix.length, end:el.selectionEnd+suffixToken.length, kind};
  }
  function showCompletion(el, force = false) {
    const context = completionContext(el);
    if (!force && !/[a-zA-Z0-9_.{"\[]$/.test(el.value.slice(0,el.selectionStart))) { closeCompletion(); return; }
    suggest = {...context, id:Number(el.dataset.editor), index:0}; paintCompletion();
  }
  function paintCompletion() {
    if (!suggest) return;
    const box = $(`[data-popup="${suggest.id}"]`), el = $(`[data-editor="${suggest.id}"]`); if (!box || !el) return;
    box.hidden = false; box.innerHTML = `<div class="argus-completion-head">${esc(suggest.kind)} · ${t('本地示例补全', 'Local fixture completion')}</div><div role="listbox" id="completion-list-${suggest.id}">${suggest.items.map((x,i) => `<button role="option" id="completion-${suggest.id}-${i}" class="argus-completion-item" aria-selected="${i === suggest.index}" data-act="complete" data-value="${i}" tabindex="-1"><span><code>${esc(x.label)}</code><small>${esc(x.desc)}</small></span><span class="argus-badge">${esc(x.tag)}</span></button>`).join('') || `<div class="argus-empty">${t('当前上下文没有匹配建议', 'No suggestions in this context')}</div>`}</div><div class="argus-completion-foot">↑ ↓ ${t('选择', 'select')} · Enter / Tab ${t('插入', 'insert')} · Esc ${t('关闭', 'close')}</div>`;
    el.setAttribute('aria-expanded','true'); el.setAttribute('aria-controls',`completion-list-${suggest.id}`); if (suggest.items.length) el.setAttribute('aria-activedescendant',`completion-${suggest.id}-${suggest.index}`);
  }
  function closeCompletion() { if (suggest) { const el = $(`[data-editor="${suggest.id}"]`); el?.setAttribute('aria-expanded','false'); el?.removeAttribute('aria-activedescendant'); el?.removeAttribute('aria-controls'); const box = $(`[data-popup="${suggest.id}"]`); if (box) box.hidden = true; } suggest = null; }
  function applyCompletion(index) {
    if (!suggest?.items[index]) return;
    const {id,start,end} = suggest, item = suggest.items[index], el = $(`[data-editor="${id}"]`);
    el.setRangeText(item.insert,start,end,'end'); if (item.cursorBack) el.setSelectionRange(el.selectionStart-item.cursorBack,el.selectionStart-item.cursorBack);
    const q = selectedPanels().find(p => p.id === id); q.expr = el.value; q.status = 'dirty'; q.revision++; closeCompletion(); el.focus();
    $(`[data-status="${id}"]`).textContent = statusText(q); $(`[data-result="${id}"]`).innerHTML = resultView(q);
    if (item.cursorBack) showCompletion(el,true);
  }
  document.addEventListener('pointerdown', e => { if (!e.target.closest('.argus-editor-wrap')) closeCompletion(); if (e.target.closest('[data-act="complete"]')) e.preventDefault(); });
  document.addEventListener('keydown', e => {
    if (S.drawer||S.picker) {
      if (e.key === 'Escape') { e.preventDefault(); if(S.picker)closeResourcePicker();else closeDrawer(); return; }
      if (e.key === 'Tab') { const els = $$('button,input,select,a', $(S.picker?'.argus-resource-modal':'.argus-drawer')).filter(x=>!x.disabled); const first = els[0], last = els.at(-1); if (e.shiftKey && document.activeElement === first) {e.preventDefault(); last.focus();} else if (!e.shiftKey && document.activeElement === last) {e.preventDefault(); first.focus();} }
    }
    if (e.target.dataset.act === 'tab' && ['ArrowLeft','ArrowRight','Home','End'].includes(e.key)) { e.preventDefault(); const ids = ['query','catalog'], n = ids.indexOf(S.tab); S.tab = e.key === 'Home' ? ids[0] : e.key === 'End' ? ids[1] : ids[(n+1)%2]; render(); $(`#tab-${S.tab}`).focus(); return; }
    if(e.target.dataset.act==='scope-kind'&&['ArrowLeft','ArrowRight','Home','End'].includes(e.key)){
      e.preventDefault();const i=resourceKinds.indexOf(e.target.dataset.value);S.picker.kind=e.key==='Home'?resourceKinds[0]:e.key==='End'?resourceKinds.at(-1):resourceKinds[(i+(e.key==='ArrowRight'?1:resourceKinds.length-1))%resourceKinds.length];S.picker.search='';render();$(`[data-act="scope-kind"][data-value="${S.picker.kind}"]`).focus();return;
    }
    if (!e.target.dataset.editor) return;
    if ((e.ctrlKey || e.metaKey) && e.code === 'Space') { e.preventDefault(); showCompletion(e.target,true); return; }
    if (e.shiftKey && e.key === 'Enter') { e.preventDefault(); run(selectedPanels().find(p => p.id === Number(e.target.dataset.editor))); return; }
    if (!suggest) return;
    if (e.key === 'Escape') { e.preventDefault(); closeCompletion(); }
    if (['ArrowDown','ArrowUp'].includes(e.key) && suggest.items.length) { e.preventDefault(); suggest.index = (suggest.index+(e.key === 'ArrowDown'?1:suggest.items.length-1))%suggest.items.length; paintCompletion(); }
    if (['Enter','Tab'].includes(e.key) && suggest.items.length) { e.preventDefault(); applyCompletion(suggest.index); }
  });
  function wireCharts() {
    $$('[data-chart]').forEach(el => {
      el.addEventListener('pointermove', e => {
        const rect = $('.argus-chart',el).getBoundingClientRect(), ratio = Math.max(0,Math.min(1,((e.clientX-rect.left)/rect.width*1010-48)/940)), i = Math.round(ratio*60);
        const mins = 660 - Number(S.range) + ratio * Number(S.range), time = `${String(Math.floor(mins/60)).padStart(2,'0')}:${String(Math.floor(mins%60)).padStart(2,'0')}`;
        (S.linked ? $$('[data-chart]') : [el]).forEach(chartEl => {const line = $('.argus-crosshair',chartEl), q = selectedPanels().find(x => x.id === Number(chartEl.dataset.chart)), m = metricFor(q); line.style.display = 'block'; line.setAttribute('x1',48+ratio*940); line.setAttribute('x2',48+ratio*940); $(`[data-readout="${q.id}"]`).textContent = `${time} · ${names(m,q).map((n,k)=>`${n}: ${values(m,k,61,q)[i].toFixed(1)} ${units(m,q)}`).join(' / ')}`;});
      });
      el.addEventListener('pointerleave', () => { $$('.argus-crosshair').forEach(line => line.style.display='none'); });
    });
  }
  render();
})();
