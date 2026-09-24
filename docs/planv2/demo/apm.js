(() => {
  'use strict';
  const D=window.ArgusDemo,M=D.model,S=D.sources;
  const tr=(zh,en)=>D.local(zh,en),e=v=>D.views.e(v);
  D.addStrings({
    'type.apm_overview':['APM 服务总览','APM service overview'],'type.apm_topology':['APM 服务拓扑','APM service topology'],'type.apm_endpoints':['APM 接口分析','APM endpoint analysis'],
    apmSamples:['以下统计仅基于当前接收到的模拟样本，不代表全量请求。','Statistics use the received sample data, not all application traffic.'],
    apmOverview:['服务与实例总览','Service & instance overview'],apmTopology:['服务调用拓扑','Service dependency map'],apmEndpoints:['接口与慢错误请求','Endpoints, slow & failed requests'],
    apmDetail:['APM 排查详情','APM investigation details'],fullscreen:['全屏查看','View fullscreen'],exitFullscreen:['退出全屏','Exit fullscreen'],
    drilldowns:['标准下钻','Standard drilldowns'],generateDrilldowns:['生成 / 重新生成标准下钻','Generate / regenerate standard drilldowns'],
    drilldownHint:['这些定义随统计图一起发布。修改后进入草稿；UI 与 AI 共用，不是导出配置。查询文本仅作模拟配置，不在浏览器执行。','Published with the panel and shared by UI and AI. Changes stay in the draft. These are not export settings; query text is not executed in this browser.'],
    drilldownMissing:['此下钻未启用，或来源已变化；请在草稿中核对后发布。','This drilldown is disabled or its source changed. Review it in a draft and publish.'],
    targetSource:['目标来源','Target source'],queryDefinition:['查询定义（模拟）','Query definition (simulated)'],enabled:['启用','Enabled'],
    relatedLogs:['查看关联日志','Inspect related logs'],traceChoice:['选择 Trace','Select trace'],
    sampleP95:['样本 P95','Sample P95'],sampleErrorRate:['样本错误率','Sample error rate'],
    apmNew:['体验最新闭环：APM、下钻与版本','Try APM, drilldowns & revisions']
  });
  function standardDrilldowns(p){
    const source=p.sourceBinding||({traces:'otel',logs:'otlp_logs',metrics:'prometheus'}[p.signal]);
    const common={enabled:true,origin:source,timePolicy:'inherit',scopePolicy:'inherit'};
    if(p.signal==='traces')return [
      {...common,id:'trace-detail',kind:'trace_detail',label:'Trace detail',signal:'traces',source,query:'query { queryTrace(traceId: "{{trace_id}}") { traceId spans { spanId parentSpanId operationName duration status } } }'},
      {...common,id:'related-logs',kind:'related_logs',label:'Related logs',signal:'logs',source:'otlp_logs',query:'trace_id = "{{trace_id}}"'}
    ];
    if(p.signal==='logs'&&M.effectiveBuilder(p)?.op!=='records')return [
      {...common,id:'log-records',kind:'log_records',label:'Log records',signal:'logs',source,query:'body: * | limit 100'}
    ];
    if(p.signal==='logs')return [
      {...common,id:'log-context',kind:'log_context',label:'Log context',signal:'logs',source,query:'service_name = "{{service}}"'},
      {...common,id:'linked-trace',kind:'trace_detail',label:'Linked trace',signal:'traces',source:'otel',query:'query { queryTrace(traceId: "{{trace_id}}") { traceId spans { spanId parentSpanId operationName duration status } } }'}
    ];
    return [];
  }
  function validate(p){
    const errors=[];
    for(const d of p.drilldowns||[]){
      if(!d.enabled)continue;
      if(d.origin!==(p.sourceBinding||({traces:'otel',logs:'otlp_logs',metrics:'prometheus'}[p.signal])))errors.push(M.label(p,'title')+': '+D.t('drilldownMissing'));
      if(!d.query?.trim()||!S.providers[d.source]||S.providers[d.source].signal!==d.signal)errors.push(M.label(p,'title')+': '+D.t('invalidQuery'));
    }
    return errors;
  }
  function definitions(p){return p.drilldowns===undefined?standardDrilldowns(p).filter(d=>d.kind!=='related_logs'):p.drilldowns}
  function allowed(p,kind){return definitions(p).find(d=>d.kind===kind&&d.enabled&&d.origin===(p.sourceBinding||({traces:'otel',logs:'otlp_logs',metrics:'prometheus'}[p.signal])))}
  function resolve(p,kind,record,ctx,variables,scene){
    const def=allowed(p,kind);if(!def)return {error:'drilldownMissing'};
    const target=M.panel('detail-'+p.id,def.signal,def.label,def.label,def.signal==='traces'?'traces':'records');
    target.sourceBinding=def.source;target.resources=p.resources;
    if(def.signal===p.signal){target.builder={...(M.effectiveBuilder(p)||target.builder),op:def.signal==='traces'?'traces':'records'};}
    else {target.builder.variable='';target.builder.environmentVariable='';target.builder.severity='ANY';}
    target.expression=M.expression(target);target.drilldowns=[];
    const detailContext=M.copy(ctx);detailContext.localFilters={};
    if(def.signal===p.signal&&kind==='log_records')detailContext.localFilters[target.id]=D.sources.filterState(p,ctx);
    const result=M.panelData(target,detailContext,variables,scene);
    let rows=result.rows;
    if(record&&['trace_detail','related_logs'].includes(kind))rows=rows.filter(r=>(r.traceId||r.id)===(record.traceId||record.id));
    if(record&&kind==='log_context')rows=rows.filter(r=>r.resource===record.resource&&r.service===record.service&&r.environment===record.environment&&Math.abs(r.age-record.age)<=5);
    return {rows,status:rows.length?result.status:'noData',definition:def,target};
  }
  function traceTarget(p,record,ctx,scene){
    const out=resolve(p,'trace_detail',record,ctx,D.currentData?.()?.variables||[],scene);
    return out.error?out:out.rows[0]?{record:out.rows[0]}:{error:out.status};
  }
  function drillLabel(d){return ({trace_detail:D.t('sourceInspect'),related_logs:D.t('relatedLogs'),log_context:D.t('contextLines'),log_records:tr('日志明细','Log records')})[d.kind]||d.label}
  function settings(p){
    if(p.signal==='metrics')return '';
    const V=D.views,defs=p.drilldowns;
    return '<details class="argus-detail"'+(D.ui.modal?.drillOpen?' open':'')+'><summary>'+e(D.t('drilldowns'))+'</summary><div class="argus-stack"><small>'+e(D.t('drilldownHint'))+'</small><button class="argus-button" data-apm-action="generate">'+e(D.t('generateDrilldowns'))+'</button>'+
      (defs||[]).map((d,i)=>'<section class="argus-card argus-card-body argus-stack"><label class="argus-check"><input type="checkbox" data-drill-field="enabled" data-drill-index="'+i+'"'+(d.enabled?' checked':'')+'>'+e(drillLabel(d))+'</label><label class="argus-field"><span>'+e(D.t('targetSource'))+'</span><select class="argus-select" data-drill-field="source" data-drill-index="'+i+'">'+Object.entries(S.providers).filter(([,v])=>v.signal===d.signal).map(([id,v])=>'<option value="'+id+'"'+(id===d.source?' selected':'')+'>'+e(v.name)+'</option>').join('')+'</select></label><small>'+e(tr('继承有效时间、资源与过滤条件','Inherit effective time, resources and filters'))+'</small><label class="argus-field"><span>'+e(D.t('queryDefinition'))+'</span><textarea class="argus-textarea argus-code-editor" data-drill-field="query" data-drill-index="'+i+'">'+e(d.query)+'</textarea></label></section>').join('')+'</div></details>';
  }
  function groups(rows,key){
    return [...new Set(rows.map(r=>r[key]||r.service))].map(name=>{
      const list=rows.filter(r=>(r[key]||r.service)===name),sorted=list.map(r=>r.duration).sort((a,b)=>a-b);
      return {name,rows:list,count:list.length,errors:list.filter(r=>r.status==='ERROR').length,p95:sorted[Math.max(0,Math.ceil(sorted.length*.95)-1)]||0,instances:new Set(list.map(r=>r.instance||r.resource)).size};
    });
  }
  function summary(rows){
    const count=rows.length,errors=rows.filter(r=>r.status==='ERROR').length;
    return '<div class="argus-source-stats"><div><strong>'+count+'</strong><small>'+e(D.t('sourceRequests'))+'</small></div><div><strong>'+(count?(100*errors/count).toFixed(1):'0')+'%</strong><small>'+e(D.t('sampleErrorRate'))+'</small></div><div><strong>'+new Set(rows.map(r=>r.service)).size+'</strong><small>'+e(D.t('sourceServices'))+'</small></div></div><small class="argus-muted">'+e(D.t('apmSamples'))+'</small>';
  }
  function supports(p){return p.signal==='traces'&&['apm_overview','apm_topology','apm_endpoints','trace_detail'].includes(p.type)}
  function content(p,result){
    const rows=result.rows;
    if(!rows.length)return '<div class="argus-empty">'+e(D.t(result.status))+'</div>';
    if(p.type==='apm_topology')return '<small class="argus-muted">'+e(D.t('apmSamples'))+'</small><div class="argus-viz-host" data-echart="'+e(p.id)+'" role="img" aria-label="'+e(M.label(p,'title'))+'"></div>';
    if(p.type==='trace_detail'){
      D.ui.traceSelections??={};const r=rows.find(x=>x.id===D.ui.traceSelections[p.id])||rows[0],spans=D.visuals.spans(r);
      return '<div class="argus-stack"><label class="argus-field"><span>'+e(D.t('traceChoice'))+'</span><select class="argus-select" data-trace-choice="'+e(p.id)+'">'+rows.map(x=>'<option value="'+e(x.id)+'"'+(x===r?' selected':'')+'>'+e(x.service+' / '+x.id)+'</option>').join('')+'</select></label>'+D.sourceViews.detailHeader(r)+'<div class="argus-trace-mini">'+spans.map(s=>'<div><span>'+e(s.service+' · '+s.operation)+'</span><span>'+s.duration+' ms</span></div>').join('')+'</div><button class="argus-button" data-viz-action="trace" data-viz-panel="'+e(p.id)+'" data-row="'+e(r.id)+'">'+e(D.t('fullscreen'))+'</button></div>';
    }
    const key=p.type==='apm_endpoints'?'operation':'service';
    return '<div class="argus-apm-result">'+summary(rows)+'<div class="argus-table-wrap argus-apm-table"><table class="argus-table"><thead><tr><th>'+e(D.t(key==='service'?'sourceService':'sourceOperation'))+'</th><th>'+e(D.t('sourceRequests'))+'</th><th>'+e(D.t('sourceInstances'))+'</th><th>'+e(D.t('sampleP95'))+'</th><th></th></tr></thead><tbody>'+
      groups(rows,key).map(g=>'<tr><td>'+e(g.name)+'</td><td>'+g.count+'</td><td>'+g.instances+'</td><td>'+g.p95+' ms</td><td><button class="argus-button argus-button--small" data-apm-action="entity" data-apm-panel="'+e(p.id)+'" data-key="'+key+'" data-value="'+e(g.name)+'">'+e(D.t('explore'))+'</button></td></tr>').join('')+'</tbody></table></div></div>';
  }
  function graphOption(p,result){
    const style=getComputedStyle(document.documentElement),color=k=>style.getPropertyValue(k).trim(),names=new Set(),edges=new Map();
    result.rows.forEach(r=>{const spans=D.visuals.spans(r);spans.forEach(s=>{names.add(s.service);const parent=spans.find(x=>x.id===s.parent);if(parent&&parent.service!==s.service){const key=parent.service+'→'+s.service;edges.set(key,{source:parent.service,target:s.service,value:(edges.get(key)?.value||0)+1})}})});
    return {animation:false,tooltip:{trigger:'item',confine:true},series:[{type:'graph',layout:'circular',roam:true,label:{show:true,color:color('--text-primary'),position:'bottom'},edgeSymbol:['none','arrow'],lineStyle:{color:color('--text-tertiary'),curveness:.14},itemStyle:{color:color('--info')},data:[...names].map(name=>({name,symbolSize:Math.max(28,parseFloat(style.getPropertyValue('--space-10'))||40)})),links:[...edges.values()]}]};
  }
  function openEntity(p,key,value){
    const data=D.currentData(),result=M.panelData(p,D.ui.ctx,data.variables,D.ui.scene);
    D.openModal({kind:'apm-entity',panel:p,result:{...result,rows:result.rows.filter(r=>(r[key]||r.service)===value||(key==='service'&&D.visuals.spans(r).some(s=>s.service===value)))},entity:value,fullscreen:false});
  }
  function isModal(kind){return ['apm-entity','apm-logs'].includes(kind)}
  function modal(u){
    const V=D.views,p=u.panel;
    let body='';
    if(u.kind==='apm-logs')body='<p>'+e(u.entity)+'</p><pre>'+e(u.rows.map(r=>r.severity+' '+r.body).join('\n')||D.t('noData'))+'</pre>';
    else body='<h3>'+e(u.entity)+'</h3>'+D.sourceViews.sourceBadge(p)+summary(u.result.rows)+'<div class="argus-apm-records">'+u.result.rows.map(r=>'<div><span>'+e(r.id)+' · '+r.duration+' ms</span><button class="argus-button argus-button--small" data-viz-action="trace" data-viz-panel="'+e(p.id)+'" data-row="'+e(r.id)+'">'+e(D.t('sourceInspect'))+'</button><button class="argus-button argus-button--small" data-apm-action="logs" data-apm-panel="'+e(p.id)+'" data-record="'+e(r.id)+'"'+(!allowed(p,'related_logs')?' disabled':'')+'>'+e(D.t('relatedLogs'))+'</button></div>').join('')+'</div>';
    const html=V.modal(D.t('apmDetail'),body,'<button class="argus-button" data-apm-action="fullscreen">'+e(D.t(u.fullscreen?'exitFullscreen':'fullscreen'))+'</button>'+V.button('close','modal-close'));
    return u.fullscreen?html.replace('class="argus-modal"','class="argus-modal argus-modal--fullscreen"'):html;
  }
  function upgrade(state){
    if(!M.actors.viewer.dashboards.includes('apm-review'))M.actors.viewer.dashboards.push('apm-review');
    if(state.dashboards.some(b=>b.id==='apm-review'))return;
    const base=M.copy(state.dashboards.find(b=>b.id==='source-workbench'));
    if(!base)return;
    base.id='apm-review';base.name='APM 与查询闭环';base.nameEn='APM & query lifecycle';base.revision=1;base.defaultRange='15m';
    base.description='服务、拓扑、接口与标准下钻；全部为模拟样本。';base.descriptionEn='Services, dependencies, endpoints and published drilldowns. Simulated samples.';
    base.panels=[
      ['apm-services','服务与实例总览','Services & instances','apm_overview','skywalking',{x:0,y:0,w:6,h:14}],
      ['apm-map','服务调用拓扑','Service dependencies','apm_topology','skywalking',{x:6,y:0,w:6,h:14}],
      ['apm-endpoints','接口与时延分析','Endpoints & latency','apm_endpoints','jaeger',{x:0,y:14,w:6,h:14}],
      ['apm-trace','单条 Trace 详情','Single trace detail','trace_detail','jaeger',{x:6,y:14,w:6,h:14}],
      ['apm-log-stat','错误日志统计','Error log count','stat','filelog',{x:0,y:28,w:4,h:12}],
      ['apm-log-trend','日志数量趋势','Log volume trend','time_series','filelog',{x:4,y:28,w:8,h:12}]
    ].map(([id,title,titleEn,type,source,layout])=>{
      const signal=S.providers[source].signal,p=M.panel(id,signal,title,titleEn,signal==='logs'?(type==='stat'?'count':'trend'):'traces',type);
      p.sourceBinding=source;p.localDefaults={...S.defaults};p.builder.variable='';p.builder.environmentVariable='environment';p.layout=layout;
      if(signal==='logs')p.builder.severity=type==='stat'?'ERROR':'ANY';
      if(type==='trace_detail')p.builder.op='detail';
      if(type.startsWith('apm_'))p.unit='';
      p.drilldowns=standardDrilldowns(p);p.expression=M.expression(p);return p;
    });
    state.dashboards.unshift(base);M.persist();
  }
  document.addEventListener('change',ev=>{
    const el=ev.target;
    if(el.dataset.traceChoice){D.ui.traceSelections??={};D.ui.traceSelections[el.dataset.traceChoice]=el.value;D.refreshPanel(el.dataset.traceChoice)}
    if(el.dataset.drillField&&el.tagName!=='TEXTAREA'){const d=D.ui.modal?.panel?.drilldowns?.[Number(el.dataset.drillIndex)];if(d)d[el.dataset.drillField]=el.type==='checkbox'?el.checked:el.value}
  });
  document.addEventListener('input',ev=>{const el=ev.target;if(el.dataset.drillField==='query'){const d=D.ui.modal?.panel?.drilldowns?.[Number(el.dataset.drillIndex)];if(d)d.query=el.value}});
  document.addEventListener('click',ev=>{
    const el=ev.target.closest('[data-apm-action]');if(!el||(el.closest('#app')&&D.ui.modal))return;
    const a=el.dataset.apmAction;
    if(a==='generate'){D.ui.modal.panel.drilldowns=standardDrilldowns(D.ui.modal.panel);D.ui.modal.drillOpen=true;D.renderModal(false);return}
    if(a==='fullscreen'){D.ui.modal.fullscreen=!D.ui.modal.fullscreen;D.renderModal(false);return}
    const p=D.currentData()?.panels.find(x=>x.id===el.dataset.apmPanel);if(!p)return;
    if(a==='entity')openEntity(p,el.dataset.key,el.dataset.value);
    if(a==='logs'||a==='log-records'){
      const data=D.currentData(),record=el.dataset.record?M.panelData(p,D.ui.ctx,data.variables,D.ui.scene).rows.find(r=>r.id===el.dataset.record):null;
      const out=resolve(p,a==='logs'?'related_logs':'log_records',record,D.ui.ctx,data.variables,D.ui.scene);
      if(out.error){D.toast(D.t(out.error));return}
      D.openModal({kind:'apm-logs',panel:p,rows:out.rows,entity:record?.id||M.label(p,'title'),fullscreen:false});
    }
  });
  D.apm={standardDrilldowns,drillLabel,validate,definitions,allowed,resolve,traceTarget,settings,groups,supports,content,graphOption,openEntity,isModal,modal,upgrade};
})();
