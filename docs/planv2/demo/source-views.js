(() => {
  'use strict';
  const D=window.ArgusDemo,M=D.model,S=D.sources;
  const tr=(zh,en)=>D.local(zh,en),esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  D.addStrings({
    reviewIntro:['{n} 个可体验变化点；最新增加厂商来源绑定、顶部与图内两层过滤、局部默认值发布。','{n} interactive changes. New: source bindings, shared and local filters, and published filter defaults.'],
    backendPending:['模拟数据与交互；正式来源识别、查询、厂商 APM 和权限仍需后端实现。','Simulated data and interactions. Source identity, queries, vendor APM and authorization require backend implementation.'],
    sourceLatest:['体验最新：多来源与两层过滤','Try sources & layered filters'],
    sourceTitle:['采集来源与局部默认值','Collection source & local defaults'],
    sourceBinding:['绑定来源','Source binding'],
    sourceUnbound:['通用样例（未绑定）','Generic sample (unbound)'],
    sourceFixture:['来源与筛选均使用独立模拟数据；不能代表厂商后端已接入。','Sources and filters use separate sample data, not connected vendor backends.'],
    sourceTop:['通用过滤 · 影响适用统计图','Shared filters · applicable panels'],
    sourceTopHint:['时间和资源对所有适用图生效；自定义变量仅影响明确引用它的查询。','Time and resources apply to compatible panels. Custom variables affect only queries that reference them.'],
    sourceLocal:['本图筛选','Panel filters'],sourceMore:['更多筛选','More filters'],
    sourceInherited:['继承顶部','Inherited'],sourceShared:['共享变量','Shared variables'],
    sourceEmpty:['没有匹配数据。顶部范围与手输条件均保留。','No matches. Shared scope and typed conditions are preserved.'],
    sourceReset:['清除本图条件','Clear panel filters'],sourceDefaults:['恢复本图默认','Restore panel defaults'],
    sourceApply:['应用本图','Apply to panel'],sourceSaveDefaults:['将当前条件存入草稿默认值','Save current filters as draft defaults'],
    sourceDraftSaved:['局部默认值已保存到个人草稿，统一发布后生效。','Local defaults saved to your draft. Publish to apply them.'],
    sourceDefaultHint:['这里修改的是发布默认值；查看页面的临时筛选不会自动保存。','These are published defaults. Temporary viewer filters are not saved automatically.'],
    sourceFieldsHint:['局部筛选按当前来源生成；候选基于顶部范围和前置条件。','Local filters follow the source. Options respect shared scope and parent filters.'],
    sourceResults:['{n} 条匹配样例','{n} matching samples'],
    sourceFallback:['{field} 的选值 {value} 已不在当前候选中，自动改为全部。','{field}: {value} is no longer available; changed to All.'],
    sourceService:['服务','Service'],sourceInstance:['服务实例','Service instance'],
    sourceEndpoint:['接口','Endpoint'],sourceOperation:['操作','Operation'],
    sourceTag:['标签条件','Tag condition'],sourceScope:['埋点库','Instrumentation scope'],
    sourceKind:['Span 类型','Span kind'],sourceSeverity:['日志级别','Log level'],
    sourceFile:['日志文件','Log file'],sourceStatus:['请求状态','Request status'],
    sourceKeyword:['关键词 / Trace ID','Keyword / trace ID'],sourceMin:['最小时延（ms）','Minimum duration (ms)'],
    sourceRequests:['请求样例','Request samples'],sourceErrors:['错误样例','Error samples'],
    sourceServices:['服务数','Services'],sourceInstances:['实例数','Instances'],
    sourceInspect:['查看链路','Inspect trace'],sourceRecord:['当前记录','Current record'],
    sourceProof:['来源范围始终保留；局部修改只刷新本图。','Source scope is retained. Local changes refresh only this panel.'],
    sourceConfig:['来源配置','Source configuration']
  });
  D.changes.push(
    {id:'source-bindings',refs:'Q35 · Q36',zh:'每张统计图独立绑定采集来源',en:'Bind a source to each panel',before:['不同厂商只有一套通用 Trace 样例。','Vendors shared a generic trace example.'],after:['同一 Dashboard 分别放置 SkyWalking、Jaeger、Prometheus 和 Filelog；每张图保留自己的来源与字段。','Separate SkyWalking, Jaeger, Prometheus and Filelog panels retain their own sources and fields.']},
    {id:'layered-filters',refs:'Q22 · Q36',zh:'顶部通用过滤与图内局部过滤',en:'Shared and panel-local filters',before:['只有顶部变量和返回结果内搜索。','Only shared variables and result-local search.'],after:['顶部时间、资源联动；图内候选和查询各自独立，条件失效自动回到全部，局部不能越过顶部范围。','Shared time and resources, independent local candidates and filters, and All fallback within the shared scope.']},
    {id:'source-defaults',refs:'Q2 · Q9 · Q36',zh:'局部默认值进入草稿统一发布',en:'Publish local defaults with the draft',before:['临时条件和默认配置没有明确区分。','Temporary and default filters were unclear.'],after:['查看时临时筛选；编辑时可保存来源和本图默认值，预览发布后其他查看者才读取新默认。','Filter temporarily in the viewer. Save sources and local defaults in a draft, then preview and publish.']}
  );
  const catalogChange=D.changes.find(c=>c.id==='catalog');
  if(catalogChange){catalogChange.after=['在明确来源和信号后发现实际字段；插件已启用与当前时段有数据分别展示。','Discover observed fields for the signal and source. Enabled plugins and data in the current window are distinct.'];catalogChange.refs='Q15 · Q35 · Q36'}
  const keyLabel=key=>D.t('source'+key[0].toUpperCase()+key.slice(1));
  const tag=(value,kind='')=>'<span class="argus-badge'+(kind?' argus-badge--'+kind:'')+'">'+esc(value)+'</span>';
  function action(key,verb,id){return '<button type="button" class="argus-button argus-button--small" data-source-action="'+verb+'" data-source-panel="'+esc(id)+'">'+esc(D.t(key))+'</button>'}
  function select(p,key,ctx,vars,scene,isDefault=false){
    const value=S.filterState(p,ctx)[key],choices=S.choices(p,key,ctx,vars,scene);
    return '<label class="argus-field"><span>'+esc(keyLabel(key))+'</span><select class="argus-select" '+(isDefault?'data-source-default':'data-source-field')+'="'+key+'" data-source-panel="'+esc(p.id)+'">'+
      ['All',...choices,...(!choices.includes(value)&&value!=='All'?[value]:[])].map(v=>'<option value="'+esc(v)+'"'+(v===value?' selected':'')+'>'+esc(v==='All'?D.t('all'):v)+'</option>').join('')+'</select></label>';
  }
  function input(p,key,state,isDefault=false){
    const pending=isDefault?{}:D.ui.sourcePending?.[p.id]||{},numeric=key==='min';
    return '<label class="argus-field"><span>'+esc((isDefault?tr('默认','Default '):'')+keyLabel(key))+'</span><input class="argus-input" '+(isDefault?'data-source-default':'data-source-field')+'="'+key+'" data-source-panel="'+esc(p.id)+'" type="'+(numeric?'number':'text')+'"'+(numeric?' min="0"':'')+' value="'+esc(pending[key]??state[key])+'"></label>';
  }
  function controls(p){
    if(!S.enabled(p))return '';
    const data=D.currentData(),ctx=D.ui.ctx,state=S.filterState(p,ctx),keys=S.definitions(p);
    const own=Object.entries(state).filter(([k,v])=>(keys.includes(k)&&v!=='All')||(k==='keyword'&&v)||(k==='min'&&p.signal==='traces'&&Number(v)>0));
    const resource=ctx.resource==='all'?D.t('allResources'):M.resources.find(r=>r.id===ctx.resource)?.name;
    const shared=data.variables.filter(v=>M.uses(p,v.key)).map(v=>M.label(v,'label')+': '+(ctx.vars[v.key]||['All']).map(x=>x==='All'?D.t('all'):x).join(', '));
    const warning=(D.ui.sourceNotices?.[p.id]||[]).map(c=>D.t('sourceFallback',{field:keyLabel(c.key),value:c.old})).join(' ');
    const advanced=keys.slice(2).map(key=>select(p,key,ctx,data.variables,D.ui.scene)).join('')+input(p,'keyword',state)+(p.signal==='traces'?input(p,'min',state):'')+action('sourceApply','apply',p.id);
    return '<section class="argus-source-filters" aria-label="'+esc(D.t('sourceLocal'))+'"><div class="argus-source-inherited">'+
      '<span>'+esc(D.t('sourceInherited'))+'</span>'+tag(D.t('range.'+ctx.range))+tag(resource)+shared.map(x=>tag(x)).join('')+'</div>'+
      '<div class="argus-source-filter-row">'+keys.slice(0,2).map(key=>select(p,key,ctx,data.variables,D.ui.scene)).join('')+'</div>'+
      '<details class="argus-source-more" data-source-more="'+esc(p.id)+'"'+(D.ui.sourceMore?.[p.id]?' open':'')+'><summary>'+esc(D.t('sourceMore'))+(own.length?' · '+own.length:'')+'</summary><div class="argus-source-advanced">'+advanced+'</div></details>'+
      (own.length?'<div class="argus-source-active">'+own.map(([k,v])=>tag(keyLabel(k)+': '+v,'accent')).join('')+'</div>':'')+
      (warning?'<small class="argus-source-warning" role="status">'+esc(warning)+'</small>':'')+
      '<div class="argus-source-actions">'+action('sourceReset','clear',p.id)+action('sourceDefaults','defaults',p.id)+
      (D.ui.view==='edit'?action('sourceSaveDefaults','save-defaults',p.id):'')+'</div></section>';
  }
  function sourceBadge(p){return S.enabled(p)?tag(S.provider(p).name,'accent'):''}
  function describeDefaults(p){
    if(!p||!S.enabled(p))return D.t('all');
    const selected=Object.entries(S.filterState(p,{})).filter(([key,value])=>value!==S.defaults[key]);
    return selected.length?selected.map(([key,value])=>keyLabel(key)+': '+value).join(' · '):D.t('all');
  }
  function traceContent(p,result){
    const isSW=p.sourceBinding==='skywalking',isJaeger=p.sourceBinding==='jaeger',rows=result.rows;
    if(!rows.length)return '<div class="argus-empty">'+esc(D.t('sourceEmpty'))+'</div>';
    const stats=[[D.t('sourceRequests'),rows.length],[D.t('sourceErrors'),rows.filter(r=>r.status==='ERROR').length],[D.t(isSW?'sourceInstances':'sourceServices'),new Set(rows.map(r=>isSW?r.instance:r.service)).size]];
    return '<div class="argus-source-trace"><div class="argus-source-stats">'+stats.map(([name,count])=>'<div><strong>'+count+'</strong><small>'+esc(name)+'</small></div>').join('')+'</div>'+
      '<div class="argus-source-count">'+esc(D.t('sourceResults',{n:rows.length}))+' · '+esc(isSW?'Segment / Endpoint':isJaeger?'Operation / Tags':'Scope / Span')+'</div>'+
      '<div class="argus-table-wrap argus-source-table"><table class="argus-table"><thead><tr><th>'+esc(D.t('sourceService'))+' / '+esc(D.t(isSW?'sourceEndpoint':isJaeger?'sourceOperation':'sourceScope'))+'</th><th>'+esc(D.t('durationLabel'))+'</th><th></th></tr></thead><tbody>'+
      rows.map(r=>'<tr data-source-record="'+esc(r.id)+'"><td><strong>'+esc(r.service)+'</strong><small class="argus-source-record-sub">'+esc(isSW?r.endpoint:isJaeger?r.operation:r.scope)+'</small><small class="argus-source-record-sub argus-mono">'+esc(isSW?r.instance+' · '+r.segmentId:isJaeger?r.tag:r.kind)+'</small></td><td>'+r.duration+' ms<br><span class="argus-level argus-level--'+(r.status==='ERROR'?'error':'info')+'">'+r.status+'</span></td><td><button class="argus-button argus-button--small" data-viz-action="trace" data-viz-panel="'+esc(p.id)+'" data-row="'+esc(r.id)+'">'+esc(D.t('sourceInspect'))+'</button></td></tr>').join('')+'</tbody></table></div></div>';
  }
  function detailHeader(r){
    if(!S.providers[r.provider])return '';
    const vendor=S.providers[r.provider],fields=r.provider==='skywalking'?{segment_id:r.segmentId,instance:r.instance,endpoint:r.endpoint}:r.provider==='jaeger'?{process:r.instance,operation:r.operation,tag:r.tag}:{scope:r.scope,kind:r.kind};
    return '<div class="argus-source-detail">'+tag(vendor.name,'accent')+tag(r.resourceName)+Object.entries(fields).map(([k,v])=>'<span><small>'+esc(k)+'</small> '+esc(v)+'</span>').join('')+'</div>';
  }
  function settings(p){
    const options=Object.entries(S.providers).filter(([,s])=>s.signal===p.signal);
    const binding='<label class="argus-field"><span>'+esc(D.t('sourceBinding'))+'</span><select class="argus-select" data-source-provider>'+(!S.enabled(p)?'<option value="">'+esc(D.t('sourceUnbound'))+'</option>':'')+options.map(([id,s])=>'<option value="'+id+'"'+(p.sourceBinding===id?' selected':'')+'>'+esc(s.name)+' · '+s.receiver+'</option>').join('')+'</select></label>';
    const ctx=M.copy(D.ui.ctx);ctx.localFilters={[p.id]:{...S.defaults,...p.localDefaults}};
    return '<section class="argus-detail argus-stack"><h3>'+esc(D.t('sourceTitle'))+'</h3>'+binding+
      (S.enabled(p)?'<small class="argus-muted">'+esc(D.t('sourceDefaultHint'))+'</small><div class="argus-form-grid">'+S.definitions(p).map(key=>select(p,key,ctx,D.currentData().variables,D.ui.scene,true)).join('')+input(p,'keyword',S.filterState(p,ctx),true)+(p.signal==='traces'?input(p,'min',S.filterState(p,ctx),true):'')+'</div>':'')+'</section>';
  }
  function topNote(data){
    if(!data.panels.some(S.enabled))return '';
    return '<div class="argus-source-top"><div><strong>'+esc(D.t('sourceTop'))+'</strong><p>'+esc(D.t('sourceTopHint'))+'</p></div><span class="argus-badge argus-badge--accent">Q35 · Q36</span></div>';
  }
  function guide(data){
    if(data.id!=='source-workbench')return '';
    return '<div class="argus-source-guide"><div><strong>'+esc(tr('本轮重点：来源分开，过滤分层','New: separate sources, layered filters'))+'</strong><p>'+esc(tr('先选顶部主机，再改某张图的服务；另一张图保持原条件。来源和默认值可在编辑草稿中调整。','Select a shared host, then change one panel’s service. Other panel filters stay intact. Edit source bindings and defaults in the draft.'))+'</p></div><small>'+esc(D.t('sourceFixture'))+'</small></div>';
  }
  function normalize(data,ctx,scene){
    const changes=S.normalize(data,ctx,scene);
    D.ui.sourceNotices??={};
    changes.forEach(c=>{D.ui.sourceNotices[c.panel]??=[];D.ui.sourceNotices[c.panel].push(c)});
    return changes;
  }
  function refresh(p,focusKey){
    const changes=S.normalizePanel(p,D.ui.ctx,D.currentData().variables,D.ui.scene);
    D.ui.sourceNotices??={};D.ui.sourceNotices[p.id]=changes;
    D.refreshPanel(p.id);
    if(D.ui.modal?.panel?.id===p.id&&D.visuals.isModal(D.ui.modal.kind)){
      D.ui.modal.result=M.panelData(p,D.ui.ctx,D.currentData().variables,D.ui.scene);D.renderModal(false);
    }
    if(focusKey)document.querySelector((D.ui.modal?'#modal-root ':'')+'[data-source-panel="'+p.id+'"][data-source-field="'+focusKey+'"]')?.focus({preventScroll:true});
  }
  function tryChange(id){
    if(!['source-bindings','layered-filters','source-defaults'].includes(id))return false;
    S.upgrade(D.state);
    if(id==='source-defaults'){
      if(!M.actor().manage){D.state.actor='editor';M.persist()}
      D.editBoard('source-workbench');
    }else D.openBoard('source-workbench');
    return true;
  }
  document.addEventListener('toggle',event=>{
    const id=event.target.dataset?.sourceMore;if(id){D.ui.sourceMore??={};D.ui.sourceMore[id]=event.target.open}
  },true);
  document.addEventListener('input',event=>{
    const el=event.target,key=el.dataset.sourceField;
    if(key&&el.tagName==='INPUT'){D.ui.sourcePending??={};D.ui.sourcePending[el.dataset.sourcePanel]??={};D.ui.sourcePending[el.dataset.sourcePanel][key]=el.type==='number'?Math.max(0,Number(el.value)||0):el.value}
    if(el.dataset.sourceDefault&&el.tagName==='INPUT'){
      const p=D.ui.modal?.panel;if(p){p.localDefaults??={...S.defaults};p.localDefaults[el.dataset.sourceDefault]=el.type==='number'?Math.max(0,Number(el.value)||0):el.value}
    }
  });
  document.addEventListener('change',event=>{
    const el=event.target,key=el.dataset.sourceField;
    if(el.hasAttribute('data-source-provider')){
      const p=D.ui.modal.panel;
      if(el.value)S.selectProvider(p,el.value);else {delete p.sourceBinding;delete p.localDefaults}
      D.ui.modal.validation=null;D.renderModal(false);return;
    }
    if(el.dataset.sourceDefault&&el.tagName==='SELECT'){
      const p=D.ui.modal.panel;p.localDefaults??={...S.defaults};p.localDefaults[el.dataset.sourceDefault]=el.value;
      if(el.dataset.sourceDefault==='service')S.definitions(p).slice(1).forEach(k=>{p.localDefaults[k]='All'});
      D.renderModal(false);return;
    }
    if(!key||el.tagName!=='SELECT')return;
    const p=D.currentData().panels.find(x=>x.id===el.dataset.sourcePanel);if(!p)return;
    D.ui.ctx.localFilters??={};D.ui.ctx.localFilters[p.id]={...S.filterState(p,D.ui.ctx),[key]:el.value};refresh(p,key);
  });
  document.addEventListener('click',event=>{
    const el=event.target.closest('[data-source-action]');if(!el||(el.closest('#app')&&D.ui.modal))return;
    const p=D.currentData().panels.find(x=>x.id===el.dataset.sourcePanel);if(!p)return;
    const action=el.dataset.sourceAction;D.ui.ctx.localFilters??={};
    if(action==='apply'){D.ui.ctx.localFilters[p.id]={...S.filterState(p,D.ui.ctx),...D.ui.sourcePending?.[p.id]};if(D.ui.sourcePending)delete D.ui.sourcePending[p.id]}
    if(action==='clear'||action==='defaults'){
      D.ui.ctx.localFilters[p.id]=action==='clear'?{...S.defaults}:{...S.defaults,...p.localDefaults};
      if(D.ui.sourcePending)delete D.ui.sourcePending[p.id];
    }
    if(action==='save-defaults'){
      const draft=M.getDraft(D.ui.dash);if(D.ui.view!=='edit'||!M.actor().manage||!draft)return;
      p.localDefaults=M.copy(S.filterState(p,D.ui.ctx));M.save(draft);
      const saved=document.getElementById('draft-save-state');if(saved)saved.textContent=D.t('savedAt',{time:D.views.stamp()});
      D.toast(D.t('sourceDraftSaved'));
    }
    refresh(p);
  });
  document.addEventListener('keydown',event=>{
    const el=event.target;if(event.key!=='Enter'||!el.dataset.sourceField||el.tagName!=='INPUT')return;
    event.preventDefault();
    const p=D.currentData().panels.find(x=>x.id===el.dataset.sourcePanel);if(!p)return;
    D.ui.ctx.localFilters??={};D.ui.ctx.localFilters[p.id]={...S.filterState(p,D.ui.ctx),...D.ui.sourcePending?.[p.id]};
    if(D.ui.sourcePending)delete D.ui.sourcePending[p.id];refresh(p,el.dataset.sourceField);
  });
  D.sourceViews={controls,sourceBadge,describeDefaults,traceContent,detailHeader,settings,topNote,guide,normalize,tryChange};
})();
