(() => {
  'use strict';
  const D=window.ArgusDemo,M=D.model,t=D.t;
  const e=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const attrs=obj=>Object.entries(obj).map(([k,v])=>' '+k+'="'+e(v)+'"').join('');
  const button=(key,action,extra={},primary=false)=>'<button type="button" class="argus-button'+(primary?' argus-button--primary':'')+'" data-action="'+action+'"'+attrs(extra)+'>'+e(t(key))+'</button>';
  const small=(text,action,extra={})=>'<button type="button" class="argus-button argus-button--small" data-action="'+action+'"'+attrs(extra)+'>'+e(text)+'</button>';
  const badge=(text,kind='')=>'<span class="argus-badge'+(kind?' argus-badge--'+kind:'')+'">'+e(text)+'</span>';
  const change=n=>badge(t('changed',{n}),'accent');
  const options=(values,selected,translate=false)=>values.map(v=>{const pair=Array.isArray(v)?v:[v,translate?t(v):v];return '<option value="'+e(pair[0])+'"'+(String(pair[0])===String(selected)?' selected':'')+'>'+e(pair[1])+'</option>'}).join('');
  const field=(label,name,value,kind='input',choices=[],extra={})=>'<label class="argus-field"><span>'+e(t(label))+'</span>'+(kind==='select'?'<select class="argus-select" data-field="'+name+'"'+attrs(extra)+'>'+options(choices,value)+'</select>':kind==='textarea'?'<textarea class="argus-textarea'+(name==='expression'?' argus-code-editor':'')+'" data-field="'+name+'"'+attrs(extra)+'>'+e(value)+'</textarea>':'<input class="argus-input" data-field="'+name+'" value="'+e(value)+'"'+attrs(extra)+'>')+'</label>';
  const notice=(text,kind='info',actions='')=>'<div class="argus-notice argus-notice--'+kind+'"><span aria-hidden="true">◉</span><div>'+e(text)+'</div>'+actions+'</div>';
  const heading=(eyebrow,title,desc,actions='')=>'<header class="argus-heading"><div><span class="argus-eyebrow">'+e(eyebrow)+'</span><h1>'+e(title)+'</h1><p>'+e(desc)+'</p></div><div class="argus-actions">'+actions+'</div></header>';
  const stamp=()=>new Date().toLocaleTimeString(D.locale,{hour:'2-digit',minute:'2-digit'});
  function shell(content){
    const ui=D.ui;
    const nav=[['review','◎'],['dashboards','◫'],['hosts','▣'],['clusters','▤'],['chat','◌']];
    return '<div class="argus-shell"><aside class="argus-sidebar"><div class="argus-brand"><span class="argus-brand-mark">◉</span><div>Argus<small>ENTERPRISE</small></div></div><div class="argus-enterprise"><strong>Acme Cloud</strong><div class="argus-muted"><small>'+e(t('prototype'))+'</small></div></div><nav class="argus-nav" aria-label="'+e(t('dashboards'))+'">'+nav.map(([id,icon])=>'<button data-action="nav" data-view="'+id+'" class="'+((ui.view===id || id==='dashboards'&&['dashboard','edit'].includes(ui.view))?'is-active':'')+'"><span class="argus-nav-icon">'+icon+'</span>'+e(t(id))+'</button>').join('')+'</nav><div class="argus-sidebar-foot">'+e(t('storageHint'))+'</div></aside><main class="argus-main'+(D.ui.focusMode?' argus-main--focus':'')+'"><header class="argus-topbar"><div class="argus-row">'+badge('PLAN V2','accent')+'<strong>'+e(t(ui.view==='dashboard'||ui.view==='edit'?'dashboards':ui.view))+'</strong></div><div class="argus-actions"><select class="argus-select" data-setting="actor" aria-label="'+e(t('admin'))+'">'+options([['editor',t('admin')],['viewer',t('viewer')]],D.state.actor)+'</select>'+small(t('language'),'locale')+small('◐','theme',{'aria-label':t('theme')})+small(t('guide'),'nav',{'data-view':'review'})+'</div></header><div class="argus-content">'+content+'</div></main></div>';
  }
  function review(){
    const count=Object.values(D.state.review).filter(Boolean).length;
    const hero='<section class="argus-review-hero argus-stack"><div class="argus-row argus-between"><span class="argus-eyebrow">PLAN V2 / DECISION REVIEW</span>'+badge(t('reviewed',{n:count,total:D.changes.length}),'success')+'</div><h1>'+e(t('titleReview'))+'</h1><p class="argus-muted">'+e(t('reviewIntro',{n:D.changes.length}))+'</p><div class="argus-row">'+button('dashboards','nav',{'data-view':'dashboards'},true)+button('reset','reset')+'</div><small class="argus-muted">'+e(t('prototypeHint'))+'</small></section>';
    return hero+(D.features?.reviewLinks()||'')+'<div class="argus-source-review-link">'+button('sourceLatest','try',{'data-id':'layered-filters'},true)+'</div><section class="argus-grid">'+D.changes.map((c,i)=>'<article class="argus-card argus-half argus-review-card"><header class="argus-card-head"><div class="argus-row">'+change(String(i+1).padStart(2,'0'))+'<h2>'+e(D.locale==='en-US'?c.en:c.zh)+'</h2></div><small class="argus-muted">'+e(D.locale==='en-US'?c.refs.replace('最新修正','Latest correction').replace('数据发现','Discovery'):c.refs)+'</small></header><div class="argus-card-body"><div class="argus-compare"><div><small class="argus-muted">'+e(t('before'))+'</small>'+e(c.before[D.locale==='en-US'?1:0])+'</div><div><small class="argus-success-text">'+e(t('after'))+'</small>'+e(c.after[D.locale==='en-US'?1:0])+'</div></div>'+button('tryIt','try',{'data-id':c.id},true)+'<label class="argus-check"><input type="checkbox" data-review="'+c.id+'"'+(D.state.review[c.id]?' checked':'')+'>'+e(t('matches'))+'</label><input class="argus-input argus-note-input" data-note="'+c.id+'" value="'+e(D.state.notes[c.id]||'')+'" placeholder="'+e(t('note'))+'" aria-label="'+e(t('note'))+'"></div></article>').join('')+'</section>';
  }
  function catalog(){
    const ui=D.ui;
    const filtered=M.boards().filter(b=>(ui.folder==='all'||b.folder===ui.folder||(ui.folder==='none'&&!b.folder)) && (M.label(b)+' '+M.label(b,'description')).toLowerCase().includes(ui.search.toLowerCase()));
    const groups=D.state.folders.concat({id:null,name:t('ungrouped'),nameEn:t('ungrouped')});
    const draftCards=Object.values(D.state.drafts).filter(d=>d.owner===M.actor().id);
    const draftSection=draftCards.length?'<section class="argus-card"><header class="argus-card-head"><h2>'+e(t('drafts'))+'</h2>'+change('01')+'</header><div class="argus-folder-body">'+draftCards.map(d=>'<div class="argus-catalog-item"><div class="argus-row argus-between">'+badge(t('draft'),'warning')+'<small>'+e(d.base===null?t('unpublished'):t('baseline',{n:d.base}))+'</small></div><h3>'+e(M.label(d.data))+'</h3>'+button('resume','edit',{'data-id':d.id},true)+'</div>').join('')+'</div></section>':'';
    return heading('TELEMETRY DASHBOARDS',t('dashboards'),t('catalogIntro'),M.actor().manage?'<button class="argus-button" data-feature="lifecycle">'+e(t('lifecycle'))+'</button>'+button('newFolder','new-folder')+button('newDashboard','new-dashboard',{},true):badge(t('readOnly'),'info'))+
      '<div class="argus-filterbar"><label class="argus-field"><span>'+e(t('search'))+'</span><input class="argus-input" id="dashboard-search" data-search="dashboard" value="'+e(ui.search)+'"></label>'+field('folder','folder',ui.folder,'select',[['all',t('allFolders')],...D.state.folders.filter(f=>f.status!=='archived').map(f=>[f.id,M.label(f)]),['none',t('ungrouped')]],{'data-store':'catalog'})+'</div>'+draftSection+
      groups.filter(g=>filtered.some(b=>b.folder===g.id)).map(g=>'<section class="argus-card"><header class="argus-card-head"><h2>'+e(M.label(g))+'</h2>'+badge(filtered.filter(b=>b.folder===g.id).length+'')+'</header><div class="argus-folder-body">'+filtered.filter(b=>b.folder===g.id).map(b=>'<button class="argus-catalog-item" data-action="open" data-id="'+b.id+'"><div class="argus-row argus-between">'+badge(t('published')+' v'+b.revision,'success')+'<small class="argus-muted">'+e(t('panels',{n:b.panels.length}))+'</small></div><h2>'+e(M.label(b))+'</h2><p class="argus-muted">'+e(M.label(b,'description'))+'</p></button>').join('')+'</div></section>').join('')+(!filtered.length?'<div class="argus-empty">'+e(t('noResults'))+'</div>':'');
  }
  function filterbar(data){
    const ctx=D.ui.ctx;
    const vars=data.variables.map(v=>{
      const opts=M.options(v,data.variables,ctx,D.ui.scene);
      const values=ctx.vars[v.key]||['All'];
      return '<div class="argus-field"><span>'+e(M.label(v,'label'))+'</span><details class="argus-popup"><summary>'+e(values.includes('All')?t('all'):values.join(', '))+' ⌄</summary><div class="argus-popup-list">'+['All',...opts].map(value=>'<label class="argus-check"><input type="'+(v.selection==='multi'?'checkbox':'radio')+'" name="filter-'+e(v.key)+'" data-filter="'+e(v.key)+'" data-value="'+e(value)+'"'+(values.includes(value)?' checked':'')+'>'+e(value==='All'?t('all'):value)+'</label>').join('')+(!opts.length?'<small class="argus-muted">'+e(t('noOptions'))+'</small>':'')+'</div></details></div>';
    }).join('');
    return '<section class="argus-filterbar">'+field('time','range',ctx.range,'select',['15m','1h','6h','24h','yesterday','custom'].map(r=>[r,t('range.'+r)]),{'data-store':'context'})+
      field('resource','resource',ctx.resource,'select',[['all',t('allResources')],...M.allowedResources().map(r=>[r.id,r.name])],{'data-store':'context'})+vars+
      field('refresh','refresh',ctx.refresh,'select',[[0,t('off')],[30,t('seconds',{n:30})],[60,t('minutes',{n:1})],[300,t('minutes',{n:5})],[900,t('minutes',{n:15})]],{'data-store':'context'})+button('refreshNow','refresh')+'</section>';
  }
  function panelCard(p,data,editing){
      const result=M.panelData(p,D.ui.ctx,data.variables,D.ui.scene),r=p.layout;
      return '<article class="argus-card argus-panel argus-positioned-panel" data-panel-id="'+e(p.id)+'" style="'+D.grid.style(r)+'"><header class="argus-card-head">'+
      (editing?'<button type="button" class="argus-drag-handle" data-grid-handle="move" data-panel-id="'+e(p.id)+'" aria-label="'+e(t('movePanel',{name:M.label(p,'title')}))+'" title="'+e(t('layoutKeyboard'))+'">⠿</button>':'')+
      '<div class="argus-panel-heading"><h2>'+e(M.label(p,'title'))+'</h2><small class="argus-muted">'+e(p.unit)+'</small></div><div class="argus-actions">'+(D.sourceViews?.sourceBadge(p)||'')+badge(p.signal.toUpperCase(),p.signal==='logs'?'warning':'info')+'</div></header><div class="argus-card-body">'+(result.status==='partial'?notice(t('partial'),'warning'):'')+(D.sourceViews?.controls(p)||'')+(D.sources?.enabled(p)&&p.signal!=='traces'?'<small class="argus-source-results">'+e(t('sourceResults',{n:result.rows.length}))+'</small>':'')+D.visuals.content(p,result)+'</div><footer class="argus-card-head"><small class="argus-muted">'+(editing?e(r.w+' × '+r.h):e(t('published'))+' v'+data.revision)+'</small><div class="argus-row">'+(D.apm?.allowed(p,'log_records')?'<button class="argus-button argus-button--small" data-apm-action="log-records" data-apm-panel="'+e(p.id)+'">'+e(D.local('日志明细','Log records'))+'</button>':'')+(p.signal==='metrics'?small(t('inspectData'),'viz-data',{'data-id':p.id}):small(t('explore'),'viz-explore',{'data-id':p.id}))+(editing?small(t('panelSettings'),'panel-edit',{'data-id':p.id}):small(t('queryRead'),'query-read',{'data-id':p.id}))+'</div></footer>'+
      (editing?'<button type="button" class="argus-resize-handle" data-grid-handle="resize" data-panel-id="'+e(p.id)+'" aria-label="'+e(t('resizePanel',{name:M.label(p,'title')}))+'" title="'+e(t('layoutKeyboard'))+'">⌟</button>':'')+'</article>';
  }
  function panels(data,editing){
    if(!data.panels.length)return '<div class="argus-card argus-empty"><h2>'+e(t('empty'))+'</h2>'+(editing?button('addPanel','panel-new',{},true):'')+'</div>';
    D.grid.ensure(data.panels);
    return (editing?notice(t('layoutHelp')):'')+'<section class="argus-dashboard-grid'+(editing?' is-editing':'')+'" data-dashboard-grid style="height:'+D.grid.height(data.panels,editing)+'px" aria-label="'+e(t('dashboards'))+'">'+data.panels.map(p=>panelCard(p,data,editing)).join('')+'<div class="argus-layout-status" aria-live="polite"></div></section>';
  }
  function dashboard(editing=false){
    const data=editing?M.getDraft(D.ui.dash)?.data:M.board(D.ui.dash);
    if(!data)return notice(t('unauthorized'),'danger');
    const draft=editing?M.getDraft(D.ui.dash):null;
    let content=heading(editing?'PERSONAL DRAFT':'PUBLISHED DASHBOARD',M.label(data),editing?t('draftHint'):t('viewNote',{n:data.revision}),editing?button('viewPublished','view-published')+button('saveDraft','save-draft')+button('preview','preview',{},true):(M.actor().manage?button(M.getDraft(data.id)?'resume':'edit','edit',{'data-id':data.id},true):badge(t('readOnly'),'info')));
    content+=(D.features?.dashboardTools(data,editing)||'');
    if(editing){
      content+='<div class="argus-preview-strip argus-row argus-between"><div class="argus-row">'+change('01')+badge(t('draft'),'warning')+'<span>'+e(draft.base===null?t('unpublished'):t('baseline',{n:draft.base}))+'</span><small id="draft-save-state" class="argus-muted">'+e(t('savedAt',{time:new Date(draft.saved).toLocaleTimeString(D.locale)}))+'</small></div>'+small(t('discard'),'discard')+'</div>';
      content+='<section class="argus-card argus-card-body argus-form-grid">'+field('name','name',M.label(data),'input',[],{'data-store':'draft'})+field('folder','folder',data.folder||'none','select',[['none',t('ungrouped')],...D.state.folders.filter(f=>f.status!=='archived').map(f=>[f.id,M.label(f)])],{'data-store':'draft'})+field('description','description',M.label(data,'description'),'textarea',[],{'data-store':'draft'})+'<div class="argus-stack">'+field('defaultTime','defaultRange',data.defaultRange,'select',['15m','1h','6h','24h'].map(r=>[r,t('range.'+r)]),{'data-store':'draft'})+field('defaultRefresh','defaultRefresh',data.defaultRefresh,'select',[[0,t('off')],[30,t('seconds',{n:30})],[60,t('minutes',{n:1})],[300,t('minutes',{n:5})],[900,t('minutes',{n:15})]],{'data-store':'draft'})+'</div></section>';
    }
    if(D.ui.guide==='fallback')content+=notice(t('guideFallback'),'info',button('switch15','switch15'));
    if(D.ui.notice)content+=notice(D.ui.notice,'warning');
    content+=(D.sourceViews?.guide(data)||'')+(D.sourceViews?.topNote(data)||'')+filterbar(data)+'<div class="argus-row argus-between"><small class="argus-muted">'+e(t('scopeNote'))+'</small><div class="argus-actions">'+(editing?button('variables','variables')+button('addPanel','panel-new',{},true):'')+button('dataCatalog','data-catalog')+'</div></div>';
    content+='<details class="argus-detail"><summary>'+e(t('demoControls'))+'</summary>'+field('sampleScene','scene',D.ui.scene,'select',['normal','no_data','unavailable','partial'].map(x=>[x,t('scene.'+x)]),{'data-store':'scene'})+'<small class="argus-muted">'+e(t('prototypeHint'))+'</small></details>'+(D.features?.sourceControls(data)||'')+panels(data,editing);
    return content;
  }
  function resourcePage(type){
    const rows=M.allowedResources().filter(r=>r.type===type);
    return heading('RESOURCE SHORTCUTS',t(type==='host'?'hosts':'clusters'),t('resourceIntro'),change('05'))+
      '<section class="argus-resource-grid">'+rows.map(r=>'<article class="argus-card"><header class="argus-card-head"><h2>'+e(r.name)+'</h2>'+badge(type==='host'?'HOST':'CLUSTER','info')+'</header><div class="argus-card-body argus-stack"><p class="argus-muted">'+e(r.address)+'</p><h3>'+e(t('binding'))+'</h3>'+((D.state.bindings[r.id]||[]).filter(id=>M.canRead(id)&&M.board(id)?.lifecycle!=='archived').map(id=>'<div class="argus-row argus-between"><span>'+e(M.label(M.board(id)))+'</span>'+small(t('openDashboard'),'resource-open',{'data-resource':r.id,'data-id':id})+'</div>').join('')||'<p class="argus-muted">'+e(t('noBindings'))+'</p>')+(M.actor().manage?button('manageBinding','bindings',{'data-id':r.id}):'')+'</div></article>').join('')+'</section>'+notice(t('bindingHelp'));
  }
  function chat(){
    const c=D.chat();
    return heading('CHAT / FILE ANALYSIS',t('chat'),t('chatIntro'),change('06'))+
      '<div class="argus-chat">'+notice(t('prototypeHint'),'warning')+
      '<div class="argus-row">'+button('mention','chat-picker')+button('aiCreate','ai-create')+button('clearChat','chat-clear')+'</div>'+
      '<details class="argus-detail"><summary>'+e(t('demoControls'))+'</summary>'+field('sampleScene','scene',D.ui.chatScene,'select',['normal','no_data','unavailable','partial'].map(x=>[x,t('scene.'+x)]),{'data-store':'chat-scene'})+'</details>'+
      (c.context?'<div class="argus-card argus-card-body">'+D.workflow.contextHtml(c.context)+'</div>':'')+
      (!c.messages.length?'<div class="argus-empty"><h2>'+e(t('chatEmpty'))+'</h2>'+button('chatGeneral','chat-quick',{'data-question':t('chatGeneral')},true)+'</div>':'')+
      c.messages.map(m=>message(m)).join('')+
      (D.ui.chatBusy?'<div class="argus-notice argus-notice--info argus-progress" role="status"><i></i>'+e(t(D.ui.chatStage))+'</div>':'')+
      '<div class="argus-chat-compose argus-stack"><textarea class="argus-textarea" id="chat-input" placeholder="'+e(t('chatPlaceholder'))+'" aria-label="'+e(t('chatPlaceholder'))+'">'+e(D.ui.chatInput||'')+'</textarea><div class="argus-row argus-between"><div class="argus-row">'+['chatGeneral','chatYesterday','chatLogs'].map(k=>small(t(k),'chat-quick',{'data-question':t(k)})).join('')+'</div><button class="argus-button argus-button--primary" data-action="chat-send"'+(D.ui.chatBusy?' disabled':'')+'>'+e(t('send'))+'</button></div></div></div>';
  }
  function message(m){
    let body='';
    if(m.kind==='user')body=e(m.text);
    else if(m.kind==='text')body=e(m.key?t(m.key):m.text);
    else if(m.kind==='choice')body='<h3>'+e(t('chooseDashboards'))+'</h3><p class="argus-muted">'+e(t('chooseThenQuery'))+'</p>'+choiceList()+button('confirmChoice','chat-choice',{},true);
else if(m.kind==='created'){const active=M.getDraft(m.id),published=M.board(m.id);body=active?'<h3>'+e(t('generated'))+'</h3><p>'+e(m.name)+'</p><div class="argus-row">'+button('openDraft','edit',{'data-id':m.id})+button('preview','preview-ai',{'data-id':m.id},true)+'</div>':published?'<h3>'+e(t('published'))+' v'+published.revision+'</h3><p>'+e(m.name)+'</p>'+button('openDashboard','open',{'data-id':m.id}):'<p>'+e(t('unavailable'))+'</p>';}
    else if(m.kind==='published')body='<p>'+e(t('publishSuccess',{n:m.revision}))+'</p>'+button('openDashboard','open',{'data-id':m.id});
    else if(m.kind==='analysis'){
      const good=m.files.filter(f=>['complete','partial'].includes(f.status)),incomplete=m.files.some(f=>f.status==='partial'||f.status==='unavailable');
      body='<div class="argus-row argus-between"><h3>'+e(t('aiConclusion'))+'</h3>'+badge(t('usedFiles',{n:new Set(good.map(f=>(f.dashboard||f.path)+'/'+f.panel)).size,total:new Set(m.files.filter(f=>f.status!=='notApplicable').map(f=>(f.dashboard||f.path)+'/'+f.panel)).size}),incomplete?'warning':'info')+'</div><p>'+e(t(good.length?'conclusion':'noConclusion'))+'</p>'+(incomplete?notice(t('notAll'),'warning'):'')+'<small class="argus-muted">'+e(m.scope)+'</small><details class="argus-detail"><summary>'+e(t('toolProcess'))+'</summary><div class="argus-stack">'+m.files.map(f=>'<div class="argus-detail"><div class="argus-row argus-between"><strong>'+e(f.title)+'</strong>'+badge(t(f.status),f.status==='complete'?'success':'warning')+'</div><small class="argus-muted">v'+f.revision+' · '+e(f.path)+'</small><pre>'+e('query → Workspace file\n'+(f.signal==='logs'?'grep -n \'"severity":"ERROR"\' ':f.signal==='metrics'?'python analyze_series.py ':'python analyze_traces.py ')+f.path)+'\n'+e(t('rows',{n:f.count}))+'</pre><details><summary>'+e(t('mockFiles'))+'</summary><pre>'+e(f.content||t(f.status))+'</pre></details></div>').join('')+'</div></details>';
    }
    return '<article class="argus-chat-message '+(m.kind==='user'?'argus-chat-message--user':'')+'"><small class="argus-muted">'+(m.kind==='user'?'YOU':'ARGUS')+'</small>'+body+'</article>';
  }
  function choiceList(){
    const selected=D.ui.chatChoice||[];
    return '<div class="argus-stack">'+M.boards().map(b=>'<label class="argus-check"><input type="checkbox" data-chat-choice="'+b.id+'"'+(selected.includes(b.id)?' checked':'')+'>'+e(M.label(b))+' '+badge('v'+b.revision)+'</label>').join('')+'</div>';
  }
  function modal(title,body,footer,wide=true){
    return '<div class="argus-modal-backdrop"><section class="argus-modal'+(wide?'':' argus-modal--small')+'" role="dialog" aria-modal="true" aria-labelledby="modal-title"><header class="argus-modal-head"><h2 id="modal-title">'+e(title)+'</h2>'+small('×','modal-close',{'aria-label':t('close')})+'</header><div class="argus-modal-body">'+body+'</div><footer class="argus-modal-foot">'+footer+'</footer></section></div>';
  }
  function builderFields(p){
    const b=p.builder,vars=D.currentData()?.variables||[],store={'data-store':'panel-builder'};
    let out=field('operation','op',b.op,'select',M.operations[p.signal].map(op=>[op,t('op.'+op)]),store);
    if(p.signal==='metrics')out+=field('metric','metric',b.metric,'select',M.catalog.metrics.map(x=>[x.name,x.name]),store)+field('groupBy','group',b.group,'select',['service','status','environment'].map(x=>[x,x]),store)+field('window','window',b.window,'select',['1m','5m','15m'].map(x=>[x,x]),store);
    if(p.signal==='logs')out+=field('severity','severity',b.severity,'select',[['ANY',t('any')],['ERROR','ERROR'],['WARN','WARN'],['INFO','INFO']],store);
    if(p.signal==='traces'&&b.op==='slow')out+=field('duration','duration',b.duration,'input',[],{...store,type:'number',min:0});
    out+=field('variableRef','variable',b.variable,'select',[['',t('none')],...vars.map(v=>[v.key,'$'+v.key])],store)+field('environment','environmentVariable',b.environmentVariable||'','select',[['',t('none')],...vars.map(v=>[v.key,'$'+v.key])],store);
    if(p.signal!=='metrics')out+=field('limit','limit',b.limit,'input',[],{...store,type:'number',min:1,max:10000});
    return '<div class="argus-form-grid">'+out+'</div><details class="argus-detail" open><summary>'+e(t('sourceQuery'))+'</summary><pre id="generated-query">'+e(M.expression(p))+'</pre></details>';
  }
  function panelModal(){
    const u=D.ui.modal,p=u.panel,store={'data-store':'panel'};
    const resourceType=p.resources.length===2?'both':p.resources[0];
    const body=(D.sourceViews?.settings(p)||'')+notice(t('modeHint'))+'<div class="argus-form-grid">'+field('panelTitle','title',M.label(p,'title'),'input',[],store)+field('signal','signal',p.signal,'select',['metrics','logs','traces'].map(s=>[s,s.toUpperCase()]),store)+field('chartType','type',p.type,'select',M.allowedTypes(p).map(v=>[v,t('type.'+v)]),store)+field('applicable','resources',resourceType,'select',[['both',t('both')],['host',t('hostOnly')],['cluster',t('clusterOnly')]],store)+field('unit','unit',p.unit,'input',[],store)+field('description','description',M.label(p,'description'),'input',[],store)+'</div><div class="argus-tabbar">'+
      '<button class="argus-button '+(p.mode==='builder'?'is-active':'')+'" id="to-builder" data-action="panel-mode" data-mode="builder"'+(!M.canBuild(p)?' disabled':'')+'>'+e(t('builder'))+'</button><button class="argus-button '+(p.mode==='dsl'?'is-active':'')+'" data-action="panel-mode" data-mode="dsl">'+e(t('dsl'))+'</button></div>'+
      '<div id="conversion-note"'+(M.canBuild(p)?' hidden':'')+'>'+notice(t('cantConvert'),'warning')+'</div>'+
      (p.mode==='builder'?builderFields(p):field('query','expression',p.expression,'textarea',[],store))+
      '<div class="argus-row">'+button('validate','validate-panel')+button('runSample','sample-panel')+'</div>'+
      (u.validation?notice(t(u.validation),u.validation==='invalidQuery'?'danger':'success'):'')+
      (D.apm?D.apm.settings(p):'')+(D.visuals?D.visuals.panelOptions(p):'')+
      (u.sample?notice(t('sampleHint'),'warning')+'<div class="argus-query-preview">'+e(t('rows',{n:M.panelData(p,D.ui.ctx,D.currentData().variables,D.ui.scene).rows.length}))+'</div>':'');
    return modal(t('panelSettings'),body,button('cancel','modal-close')+(!u.isNew?button('remove','panel-remove'):'')+button('savePanel','panel-save',{},true));
  }
  function variablesModal(){
    const u=D.ui.modal,vars=u.variables,v=vars[u.index],store={'data-store':'variable'},ctx=M.copy(D.ui.ctx);
    M.normalizeContext(ctx,vars,D.ui.scene);
    let form='';
    if(v){
      const items=M.catalog[v.signal],item=items.find(x=>x.name===v.item)||items[0],fields=[...new Set(items.flatMap(x=>x.fields))].filter(x=>['environment','service','severity','status'].includes(x));
      const opts=M.options(v,vars,ctx,D.ui.scene),uses=D.currentData().panels.filter(p=>M.uses(p,v.key));
      form='<div class="argus-form-grid">'+field('key','key',v.key,'input',[],store)+field('label','label',M.label(v,'label'),'input',[],store)+field('signal','signal',v.signal,'select',['metrics','logs','traces'].map(x=>[x,x.toUpperCase()]),store)+field('sourceItem','item',v.item,'select',items.map(x=>[x.name,x.name]),store)+field('field','field',v.field,'select',fields.map(x=>[x,x]),store)+field('dependency','dependency',v.dependency,'select',[['',t('none')],...vars.filter(x=>x!==v).map(x=>[x.key,'$'+x.key])],store)+field('selection','selection',v.selection,'select',[['single',t('single')],['multi',t('multi')]],store)+field('defaultValue','defaultValues',v.defaultValues[0]||'All','select',[['All',t('all')],...opts.map(x=>[x,x])],store)+field('variableRefresh','refresh',v.refresh,'select',[['data',t('withData')],['manual',t('manual')]],store)+'</div>'+
        notice(t('forcedRefresh'))+'<section class="argus-card argus-card-body argus-stack"><h3>'+e(t('optionsPreview'))+'</h3><div class="argus-row">'+badge(t('all'),'accent')+opts.map(x=>badge(x)).join('')+'</div>'+(!opts.length?'<small>'+e(t('noOptions'))+'</small>':'')+'</section>'+
        '<section class="argus-card argus-card-body argus-stack"><h3>'+e(t('usage'))+'</h3>'+(uses.map(p=>'<div>'+badge(p.signal.toUpperCase())+' '+e(M.label(p,'title'))+'</div>').join('')||'<p class="argus-muted">'+e(t('unused'))+'</p>')+'</section>';
    }
    return modal(t('variableTitle'),'<p class="argus-muted">'+e(t('variableHelp'))+'</p>'+ (u.error?notice(t(u.error),'danger'):'')+'<div class="argus-variable-layout"><div class="argus-variable-list">'+vars.map((x,i)=>'<button class="argus-button '+(i===u.index?'argus-button--primary':'')+'" data-action="variable-select" data-index="'+i+'">$'+e(x.key)+'<br><small>'+e(M.label(x,'label'))+'</small></button>').join('')+button('addVariable','variable-add')+(v?button('remove','variable-remove'):'')+'</div><div class="argus-stack">'+form+'</div></div>',button('cancel','modal-close')+button('saveVariables','variables-save',{},true));
  }
  function diffRows(old,data){
    const rows=D.workflow.diff(old,data);
    return rows.length?'<table class="argus-table"><thead><tr><th>'+e(t('changes'))+'</th><th>'+e(t('beforeValue'))+'</th><th>'+e(t('afterValue'))+'</th></tr></thead><tbody>'+rows.map(r=>'<tr>'+r.map(c=>'<td class="argus-diff-cell">'+e(c)+'</td>').join('')+'</tr>').join('')+'</tbody></table>':'<p>'+e(t('noChanges'))+'</p>';
  }
  function previewModal(){
    const u=D.ui.modal,p=u.preview,current=M.board(p.id);
    return modal(t('previewTitle'),'<p class="argus-muted">'+e(t('previewHelp'))+'</p>'+notice(t('reasonDemo'),'warning')+
      diffRows(current,p.data)+'<div class="argus-form-grid"><section class="argus-card argus-card-body argus-stack"><h3>'+e(t('hardChecks'))+'</h3>'+(p.errors.length?p.errors.map(x=>'<p class="argus-danger-text">'+e(t(x))+'</p>').join(''):'<p class="argus-success-text">'+e(t('simValidation'))+'</p>')+'</section><section class="argus-card argus-card-body argus-stack"><h3>'+e(t('sampleChecks'))+'</h3><p>'+e(t('scene.'+p.scene))+'</p>'+(p.scene!=='normal'?'<small class="argus-warning-text">'+e(t('warningsAllowed'))+'</small>':'')+'</section></div>'+
      '<details class="argus-detail"><summary>'+e(t('query'))+'</summary>'+p.data.panels.map(x=>'<h3>'+e(M.label(x,'title'))+'</h3><pre>'+e(M.expression(x))+'</pre>').join('')+'</details>'+
      (u.error?notice(t(u.error),'danger'):'')+
      (current?'<div class="argus-row">'+button('simulateConflict','simulate-conflict')+'<small class="argus-muted">'+e(t('demoOnly'))+'</small></div>':''),
      button('cancel','modal-close')+'<button class="argus-button argus-button--primary" data-action="publish"'+(p.errors.length?' disabled':'')+'>'+e(t('publish'))+'</button>');
  }
  function catalogModal(){
    const u=D.ui.modal,items=M.catalog[u.signal].filter(x=>u.source==='all'||x.source===u.source),sources=[...new Set(M.catalog[u.signal].map(x=>x.source))];
    const body='<p class="argus-muted">'+e(t('catalogHelp'))+'</p><div class="argus-tabbar">'+['metrics','logs','traces'].map(x=>'<button class="argus-button '+(u.signal===x?'is-active':'')+'" data-action="catalog-signal" data-signal="'+x+'">'+x.toUpperCase()+'</button>').join('')+'</div>'+field('source','source',u.source,'select',[['all',t('allSources')],...sources.map(x=>[x,x])],{'data-store':'catalog-modal'})+
      items.map(x=>'<details class="argus-detail"><summary><strong>'+e(x.name)+'</strong> '+badge(x.type,'info')+' <small>'+e(x.source)+'</small></summary><div class="argus-stack">'+x.fields.map(f=>'<div class="argus-row argus-between"><div><code>'+e(f)+'</code> '+M.options({key:'catalog',field:f,dependency:''},[],D.ui.ctx,D.ui.scene).map(v=>badge(v)).join('')+'</div>'+(D.ui.view==='edit'?small(t('useField'),'catalog-use',{'data-signal':u.signal,'data-item':x.name,'data-field-name':f}):'')+'</div>').join('')+'</div></details>').join('')+'<small class="argus-muted">'+e(t('catalogReadonly'))+'</small>';
    return modal(t('dataCatalog'),body,button('close','modal-close'));
  }
  function dialog(){
    const u=D.ui.modal;if(!u)return '';
    if(D.features?.isModal(u.kind))return D.features.modal(u);
    if(D.apm?.isModal(u.kind))return D.apm.modal(u);
    if(D.visuals?.isModal(u.kind))return D.visuals.modal(u);
    if(u.kind==='panel')return panelModal();
    if(u.kind==='variables')return variablesModal();
    if(u.kind==='preview')return previewModal();
    if(u.kind==='catalog')return catalogModal();
    if(u.kind==='create')return modal(t('newDashboard'),notice(t('draftHint'))+field('name','name',u.name,'input',[],{'data-store':'create'})+field('description','description',u.description,'textarea',[],{'data-store':'create'})+field('folder','folder',u.folder,'select',[['none',t('ungrouped')],...D.state.folders.filter(f=>f.status!=='archived').map(f=>[f.id,M.label(f)])],{'data-store':'create'}),button('cancel','modal-close')+button('createDraft','create-draft',{},true),false);
    if(u.kind==='folder')return modal(t('newFolder'),field('name','name',u.name,'input',[],{'data-store':'folder'}),button('cancel','modal-close')+button('apply','create-folder',{},true),false);
    if(u.kind==='query')return modal(t('queryRead'),'<h3>'+e(M.label(u.panel,'title'))+'</h3>'+badge(u.panel.signal.toUpperCase(),'info')+(D.sourceViews?.sourceBadge(u.panel)||'')+(D.sources?.enabled(u.panel)?'<h3>'+e(t('sourceDefaultHint'))+'</h3><pre>'+e(JSON.stringify(u.panel.localDefaults||{},null,2))+'</pre>':'')+'<pre class="argus-query-preview">'+e(M.expression(u.panel))+'</pre>'+notice(t('readOnly')),button('close','modal-close'));
    if(u.kind==='time')return modal(t('time'),field('from','from',u.from,'input',[],{'data-store':'time',type:'datetime-local'})+field('to','to',u.to,'input',[],{'data-store':'time',type:'datetime-local'}),button('cancel','modal-close')+button('apply','time-apply',{},true),false);
    if(u.kind==='bindings')return modal(t('manageBinding')+' · '+M.resources.find(r=>r.id===u.resource).name,notice(t('bindingHelp'))+M.boards().map(b=>'<label class="argus-check"><input type="checkbox" data-binding="'+b.id+'"'+(u.ids.includes(b.id)?' checked':'')+'>'+e(M.label(b))+'</label>').join(''),button('cancel','modal-close')+button('bindingPreview','binding-preview',{},true),false);
    if(u.kind==='binding-preview')return modal(t('bindingPreview'),notice(t('bindingHelp'))+'<div class="argus-stack">'+u.ids.map(id=>'<p>'+e(M.label(M.board(id)))+'</p>').join('')+'</div>',button('cancel','modal-close')+button('bindingConfirm','binding-confirm',{},true),false);
    if(u.kind==='chat-picker')return modal(t('chooseDashboards'),choiceList(),button('cancel','modal-close')+button('confirmChoice','chat-choice',{},true),false);
    if(u.kind==='ai-create')return modal(t('aiCreate'),'<p>'+e(t('createAIHelp'))+'</p>'+field('name','name',u.name,'input',[],{'data-store':'ai-create'}),button('cancel','modal-close')+button('createDSL','ai-generate',{'data-mode':'dsl'})+button('createBuilder','ai-generate',{'data-mode':'builder'},true),false);
    if(u.kind==='conflict'){
      const draft=M.getDraft(D.ui.dash),current=M.board(D.ui.dash);
      return modal(t('conflictTitle'),notice(t('conflict'),'warning')+diffRows(current,draft.data)+'<label class="argus-check"><input type="radio" name="resolve" value="published" data-resolve checked>'+e(t('keepPublished'))+'</label><label class="argus-check"><input type="radio" name="resolve" value="mine" data-resolve>'+e(t('keepMine'))+'</label>',button('cancel','modal-close')+button('resolve','resolve-conflict',{},true));
    }
    if(u.kind==='trace')return modal(t('traceDetails'),'<pre class="argus-query-preview">'+e(JSON.stringify(u.record,null,2))+'</pre>'+notice(t('sampleHint'),'warning'),button('close','modal-close'),false);
    if(u.kind==='reset')return modal(t('reset'),'<p>'+e(t('resetConfirm'))+'</p>'+notice(t('demoOnly')),button('cancel','modal-close')+button('reset','reset-confirm',{},true),false);
    return '';
  }
  D.views={panelCard,e,button,small,field,badge,notice,modal,shell,review,catalog,dashboard,resourcePage,chat,dialog,stamp};
})();
