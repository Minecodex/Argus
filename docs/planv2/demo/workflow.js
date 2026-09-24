(() => {
  'use strict';
  const D=window.ArgusDemo,M=D.model;
  const local=(zh,en)=>D.local(zh,en),same=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
  D.addStrings({
    authoringMode:['编辑模式','Editing mode'],
    latestRevision:['重新取数使用最新发布版；每张仪表盘保留独立条件。','New queries use the latest published revision, with a separate context for each dashboard.'],
    revisionChanged:['{name} 已从 v{old} 更新至 v{next}；旧文件保留原版本。','{name} changed from v{old} to v{next}; existing files retain their original revision.'],
    changedContract:['{name} 的来源或筛选含义发生变化，请重新明确条件。','The source or filter meaning changed for {name}. Please specify the conditions again.'],
    archive:['归档','Archive'],restore:['恢复','Restore'],lifecycle:['归档与恢复','Archive & restore'],
    lifecycleHint:['归档保留历史和个人草稿；恢复后重新核对发布基线。','Archiving retains history and personal drafts. Review the publication baseline after restoring.'],
    moveUngrouped:['移到未分组','Move to ungrouped'],folderNotEmpty:['分组仍有仪表盘，请先迁出。','Move dashboards out of this folder before archiving it.'],
    renewBase:['重新核对当前基线','Review current baseline'],staleBase:['对象或发布版本已变化。草稿保留，请核对新的基线后再发布。','The object or revision changed. Your draft is retained; review the new baseline before publishing.'],
    archived:['已归档','Archived'],lifecyclePreview:['预览对象变更','Preview object change'],
    lifecycleDone:['对象状态已更新，历史与草稿保留。','Object state updated. History and drafts are retained.'],
    allDetails:['完整变更明细','Full change details']
  });
  function variableContract(v){return JSON.stringify([v.signal,v.item,v.field,v.value_type||'string',v.selection,v.mapping||null,v.discovery_scope||v.source_provenance||null])}
  function panelContract(p){return JSON.stringify([M.queryContext(p),D.sources?.definitions(p),p.resources])}
  function overridesFrom(question,boards){
    const out={vars:{},locals:{}};
    if(/昨天|yesterday/i.test(question))out.range='yesterday';
    else if(/15\s*(分钟|min)/i.test(question))out.range='15m';
    else if(/24\s*(小时|h)|一天|one day/i.test(question))out.range='24h';
    else if(/最近.*小时|last hour|1\s*hour/i.test(question))out.range='1h';
    const r=M.resources.find(r=>question.includes(r.name)||r.id==='host-a'&&/主机\s*A|host A/i.test(question)||r.id==='host-b'&&/主机\s*B|host B/i.test(question));
    if(r)out.resource=r.id;
    if(/staging|预发布/i.test(question))out.vars.environment=['staging'];
    else if(/production|生产环境/i.test(question))out.vars.environment=['production'];
    if(/只看日志|logs only/i.test(question))out.panelSignal='logs';
    else if(/只看.*trace|traces only/i.test(question))out.panelSignal='traces';
    else if(/只看.*(指标|cpu)|metrics only/i.test(question))out.panelSignal='metrics';
    else if(/有没有问题|any problems/i.test(question))out.panelSignal='';
    const service=question.match(/(?:服务|service)\s*[:：=]?\s+([a-z][a-z0-9-]+)/i);
    if(service){
      const candidates=boards.flatMap(b=>b.panels.filter(p=>D.sources?.enabled(p)&&(question.includes(M.label(p,'title'))||question.toLowerCase().includes(p.sourceBinding))).map(p=>({b,p})));
      if(!candidates.length)out.clarification=local('请指出要筛选哪张统计图的服务，例如“Jaeger 服务 payment-api”。','Specify a panel source, for example “Jaeger service payment-api”.');
      candidates.forEach(({b,p})=>{out.locals[b.id]??={};out.locals[b.id][p.id]={service:service[1]}});
    }
    return out;
  }
  function plan(boards,previous,question,scene='normal'){
    const overrides=overridesFrom(question,boards),entries={},notices=[],fallbacks=[];
    if(overrides.clarification)return {clarification:overrides.clarification};
    if(overrides.resource&&!M.actor().resources.includes(overrides.resource))return {error:'unauthorized'};
    for(const b of boards){
      if(b.lifecycle==='archived'||!M.canRead(b.id))return {error:'unavailable'};
      for(const key of Object.keys(overrides.vars))if(!b.variables.some(v=>v.key===key))return {clarification:local('仪表盘“','Dashboard “')+M.label(b)+local('”未配置变量 ','” has no variable ')+key+local('，请改用资源条件或调整发布配置。','. Choose a resource condition or update its published configuration.')};
      const old=previous?.dashboards?.[b.id],ctx=M.context(b),explicit=M.copy(old?.explicit||{vars:{},locals:{}});
      explicit.vars??={};explicit.locals??={};
      if(old&&old.revision!==b.revision)notices.push(D.t('revisionChanged',{name:M.label(b),old:old.revision,next:b.revision}));
      for(const key of Object.keys(explicit.vars)){
        const v=b.variables.find(v=>v.key===key);
        if(!overrides.vars[key]&&(!v||old.variableContracts?.[key]!==variableContract(v)))return {clarification:D.t('changedContract',{name:M.label(b)+' / '+key})};
      }
      for(const id of Object.keys(explicit.locals)){
        const p=b.panels.find(p=>p.id===id);
        if(!overrides.locals[b.id]?.[id]&&(!p||old.panelContracts?.[id]!==panelContract(p)))return {clarification:D.t('changedContract',{name:M.label(b)+' / '+id})};
      }
      for(const key of ['range','resource','panelSignal'])if(key in overrides)explicit[key]=overrides[key];
      Object.assign(explicit.vars,overrides.vars);
      Object.assign(explicit.locals,overrides.locals[b.id]||{});
      for(const key of ['range','resource','panelSignal'])if(key in explicit)ctx[key]=explicit[key];
      if(ctx.resource!=='all'&&!M.actor().resources.includes(ctx.resource))return {error:'unauthorized'};
      b.variables.forEach(v=>{if(explicit.vars[v.key])ctx.vars[v.key]=M.copy(explicit.vars[v.key])});
      ctx.localFilters=M.copy(explicit.locals);
      fallbacks.push(...M.normalizeContext(ctx,b.variables,scene).map(c=>({...c,name:M.label(b)+' / '+c.name})));
      D.sources?.normalize(b,ctx,scene);
      for(const key of Object.keys(explicit.vars))if(ctx.vars[key])explicit.vars[key]=M.copy(ctx.vars[key]);
      for(const id of Object.keys(explicit.locals))if(ctx.localFilters[id])explicit.locals[id]=Object.fromEntries(Object.keys(explicit.locals[id]).map(key=>[key,M.copy(ctx.localFilters[id][key])]));
      ctx.sourceSnapshot=M.copy(D.state.sourceLifecycle||{});
      entries[b.id]={revision:b.revision,name:M.label(b),ctx,explicit,variableContracts:Object.fromEntries(b.variables.map(v=>[v.key,variableContract(v)])),panelContracts:Object.fromEntries(b.panels.map(p=>[p.id,panelContract(p)]))};
    }
    return {context:{dashboards:entries},entries,inherited:!!previous?.dashboards,notices,fallbacks};
  }
  function contextHtml(context){
    const V=D.views,entries=context?.dashboards;
    if(!entries)return '<small class="argus-muted">'+V.e(D.t('latestRevision'))+'</small>';
    return '<div class="argus-stack"><small>'+V.e(D.t('latestRevision'))+'</small>'+Object.entries(entries).map(([id,x])=>'<div class="argus-row">'+V.badge(x.name+' v'+x.revision,'accent')+V.badge(D.t('range.'+x.ctx.range))+V.badge(x.ctx.resource==='all'?D.t('allResources'):M.resources.find(r=>r.id===x.ctx.resource)?.name)+Object.entries(x.ctx.vars).map(([k,v])=>V.badge(k+': '+v.join(', '))).join('')+'</div>').join('')+'</div>';
  }
  function detailFiles(data,ctx,question,scene,normalFiles){
    const id=question.match(/\b(trace-[a-z0-9-]+)\b/i)?.[1];
    if(!id||!/(关联日志|related logs|详情|detail)/i.test(question))return [];
    const source=Object.keys(D.sources.providers).find(k=>D.sources.providers[k].signal==='traces'&&question.toLowerCase().includes(k));
    const p=data.panels.find(p=>p.signal==='traces'&&(!source||p.sourceBinding===source));
    if(!p)return [];
    const record=M.panelData(p,ctx,data.variables,scene).rows.find(r=>r.traceId===id);
    const kind=/关联日志|related logs/i.test(question)?'related_logs':'trace_detail';
    const result=record?D.apm.resolve(p,kind,record,ctx,data.variables,scene):{rows:[],status:'noData'};
    const rows=result.error?[]:kind==='trace_detail'?result.rows.flatMap(r=>D.visuals.spans(r)):result.rows;
    return [{dashboard:data.id,panel:p.id,title:M.label(p,'title')+' / '+D.t(kind==='related_logs'?'relatedLogs':'sourceInspect'),signal:kind==='related_logs'?'logs':'traces',source:result.definition?.source||p.sourceBinding,
      query:result.definition?.query||'',drilldownRef:result.definition?.id||'',revision:data.revision,time:normalFiles[0]?.time,
      status:result.error?'unavailable':result.status,count:rows.length,total:rows.length,path:'/workspace/telemetry/'+ctx.executionId+'/'+data.id+'/'+p.id+'-'+kind+'-'+id+'.jsonl',content:rows.map(r=>JSON.stringify(r)).join('\n')}];
  }
  function pretty(value){return value===undefined||value===null?'—':Array.isArray(value)?value.join(', '):typeof value==='object'?JSON.stringify(value):String(value)}
  function diff(old,data){
    const rows=[],t=D.t,add=(name,a,b,fmt=pretty)=>{if(!same(a,b))rows.push([name,fmt(a),fmt(b)])};
    const refresh=v=>v?D.t('seconds',{n:v}):t('off');
    for(const k of ['name','description'])add(t(k),old?.[k],data[k]);
    add(t('folder'),old?.folder,data.folder,v=>v?M.label(D.state.folders.find(f=>f.id===v)||{name:v}):t('ungrouped'));
    add(t('defaultTime'),old?.defaultRange,data.defaultRange,v=>v?t('range.'+v):'—');
    add(t('defaultRefresh'),old?.defaultRefresh,data.defaultRefresh,refresh);
    const ids=new Set([...(old?.panels||[]).map(p=>p.id),...data.panels.map(p=>p.id)]);
    for(const id of ids){
      const a=old?.panels.find(p=>p.id===id),b=data.panels.find(p=>p.id===id),name=M.label(b||a,'title');
      if(!a||!b){rows.push([t('panelSettings')+' · '+name,a?local('存在','Present'):'—',b?local('存在','Present'):'—']);continue}
      for(const [key,label] of [['title','panelTitle'],['description','description'],['signal','signal'],['type','chartType'],['mode','authoringMode'],['resources','applicable'],['unit','unit'],['sourceBinding','sourceBinding']])add(name+' · '+t(label),a[key],b[key],v=>key==='type'?t('type.'+v):key==='mode'?t(v==='dsl'?'dsl':'builder'):key==='sourceBinding'?D.sources?.providers[v]?.name||pretty(v):pretty(v));
      add(name+' · '+t('query'),M.expression(a),M.expression(b));
      add(name+' · '+t('defaults'),a.localDefaults,b.localDefaults,pretty);
      add(name+' · '+t('visualOptions'),a.display,b.display);
      add(name+' · '+local('布局','Layout'),a.layout,b.layout,v=>v?'x='+v.x+', y='+v.y+', '+v.w+' × '+v.h:'—');
      add(name+' · '+local('标准下钻','Standard drilldowns'),a.drilldowns,b.drilldowns,v=>(v||[]).map(d=>(D.apm?.drillLabel(d)||d.label)+' → '+(D.sources?.providers[d.source]?.name||d.source)+' / '+d.query+(d.enabled?'':' ['+local('关闭','off')+']')).join('\n')||'—');
      if(same(M.expression(a),M.expression(b)))add(name+' · '+local('构建器配置','Builder configuration'),a.builder,b.builder);
    }
    const keys=new Set([...(old?.variables||[]).map(v=>v.key),...data.variables.map(v=>v.key)]);
    for(const key of keys){
      const a=old?.variables.find(v=>v.key===key),b=data.variables.find(v=>v.key===key);
      if(!a||!b){rows.push([t('variables')+' · $'+key,a?pretty(a):'—',b?pretty(b):'—']);continue}
      for(const field of new Set([...Object.keys(a),...Object.keys(b)]))add('$'+key+' · '+({defaultValues:t('defaultValue'),field:t('field'),dependency:t('dependency'),refresh:t('variableRefresh'),selection:t('selection')}[field]||field),a[field],b[field]);
    }
    return rows;
  }
  function archivePlan(kind,id){
    if(!M.actor().manage)return {error:'unauthorized'};
    const folder=kind==='folder',object=folder?D.state.folders.find(f=>f.id===id):M.board(id);
    if(!object)return {error:'unavailable'};
    if(folder&&D.state.dashboards.some(b=>b.folder===id))return {error:'folderNotEmpty'};
    return {kind,id,expected:object.objectVersion||0,label:M.label(object),next:(folder?object.status:object.lifecycle)==='archived'?'active':'archived'};
  }
  function lifecycleCommit(plan){
    if(plan.error||!M.actor().manage)return {error:plan.error||'unauthorized'};
    const object=plan.kind==='folder'?D.state.folders.find(f=>f.id===plan.id):M.board(plan.id);
    if(!object)return {error:'unavailable'};
    if((object.objectVersion||0)!==plan.expected)return {error:'conflict'};
    if(plan.kind==='folder'&&D.state.dashboards.some(b=>b.folder===plan.id))return {error:'folderNotEmpty'};
    if(plan.kind==='move')object.folder=null;
    else if(plan.kind==='folder')object.status=plan.next;
    else object.lifecycle=plan.next;
    object.objectVersion=(object.objectVersion||0)+1;M.persist();return {ok:true};
  }
  D.workflow={plan,contextHtml,detailFiles,diff,archivePlan,lifecycleCommit,panelContract,variableContract};
})();
