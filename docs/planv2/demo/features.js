(() => {
  'use strict';
  const D=window.ArgusDemo,M=D.model;
  const tr=(zh,en)=>D.local(zh,en),V=()=>D.views,e=v=>V().e(v);
  D.addStrings({
    reviewIntro:['{n} 个可体验变化点；最新补齐配置校验、APM 与标准下钻、最新版本取数、来源历史和归档恢复。','{n} interactive changes: validation, APM and drilldowns, latest-revision queries, source history, and archive/restore.'],
    lifecycleConfirm:['确认变更','Confirm change'],
    sourceSimulation:['来源生命周期（模拟）','Source lifecycle (simulation)'],stopSource:['模拟停采','Simulate stopping'],restartSource:['模拟重装','Simulate reinstall'],
    sourceRunning:['采集中','Collecting'],sourceStopped:['已停采，保留历史','Stopped; history retained'],sourceGeneration:['当前代次 {n}','Current generation {n}'],
    sourceHistoryHint:['停采不会隐藏已接收样本。重装模拟新增请求，旧记录仍归属旧代次，新旧实例分开显示。','Stopping never hides received samples. Reinstalling adds sample requests while retaining the original source generation of old records.'],
    sourceResetDemo:['恢复来源演示状态','Reset source simulation'],nextVersion:['模拟发布下一版本','Simulate next revision'],
    nextVersionHint:['只更新此浏览器中的 APM 示例版本，用于核对 Chat 追问使用最新版本；不改现有文件。','Updates only this local APM example revision to test follow-up queries. Existing files remain unchanged.'],
    boardState:['仪表盘状态','Dashboard state'],folderState:['分组状态','Folder state']
  });
  D.changes.push(
    {id:'correct-config',refs:'UI-01～05',zh:'配置校验、图型与完整发布差异',en:'Validation, chart types & complete diffs',before:['变量前缀误匹配、来源转换恢复旧查询，部分图型与发布差异未生效。','Variable prefix collisions, stale source conversions, missing renderers and incomplete diffs.'],after:['按完整引用校验；来源变化使旧转换失效；日志图型实际切换，查询、变量、布局和刷新逐项对照。','Exact references, source-aware conversion, working log charts and field-level publication diffs.']},
    {id:'apm-drilldowns',refs:'Q39 · Q41',zh:'APM 专用图与已发布下钻',en:'APM panels & published drilldowns',before:['主要只有 Trace 列表和固定跳转。','Mostly trace lists and fixed navigation.'],after:['服务总览、拓扑、接口、Trace 详情可配置；标准下钻随图发布，详情可全屏。','Configure service, topology, endpoint and trace panels; publish drilldowns and expand details fullscreen.']},
    {id:'latest-chat',refs:'Q40 · UI-06',zh:'Chat 使用最新版本、各图独立条件',en:'Latest revisions and separate query contexts',before:['多个仪表盘共用第一张的默认条件。','Several dashboards shared the first dashboard’s defaults.'],after:['每张独立使用默认时间和变量，新取数读取最新发布版，旧文件保留原版本。','Each dashboard uses its own defaults. New queries use the latest revision; old files keep their source version.']},
    {id:'source-history',refs:'Q37 · Q38',zh:'动态来源、停采历史与重装身份',en:'Dynamic sources and retained history',before:['仅按静态来源标签展示。','Only static source labels.'],after:['按来源类型与顶部资源匹配，模拟停采和重装，历史仍可查，新旧实例分开。','Match source type and shared resources; simulate stopping or reinstalling while retaining distinct historical instances.']},
    {id:'archive-recovery',refs:'Q42',zh:'归档保留草稿，恢复后重验基线',en:'Retain drafts through archive & restore',before:['缺少归档、恢复和发布基线变化的交互。','No archive, restore or lifecycle baseline flow.'],after:['非空分组先迁出；归档不删草稿，恢复后明确重新核对基线。','Move dashboards out before archiving folders. Retain drafts and review the baseline after restoring.']}
  );
  function reviewLinks(){return '<div class="argus-row">'+V().button('apmNew','feature-open-apm',{},true)+'</div>'}
  function dashboardTools(data,editing){
    let out='';
    if(data.id==='apm-review')out+='<div class="argus-notice argus-notice--info"><div><strong>'+e(tr('Q37～Q42 · 最新闭环样例','Q37–Q42 · latest workflow example'))+'</strong><p>'+e(D.t('apmSamples'))+'</p></div>'+V().button(D.ui.focusMode?'exitFullscreen':'fullscreen','feature-fullscreen')+'</div>';
    const draft=editing?M.getDraft(data.id):null,current=M.board(data.id);
    if(draft&&current&&(draft.base!==current.revision||(draft.baseObjectVersion||0)!==(current.objectVersion||0)))out+=V().notice(D.t('staleBase'),'warning','<button class="argus-button" data-feature="rebase">'+e(D.t('renewBase'))+'</button>');
    if(M.actor().manage&&!editing)out+='<div class="argus-row">'+(data.id==='apm-review'?'<button class="argus-button" data-feature="next-version">'+e(D.t('nextVersion'))+'</button>':'')+'<button class="argus-button" data-feature="lifecycle">'+e(D.t('lifecycle'))+'</button></div>';
    return out;
  }
  function sourceControls(data){
    if(!M.actor().manage||!data.panels.some(p=>p.sourceBinding))return '';
    const ids=[...new Set(data.panels.map(p=>p.sourceBinding).filter(Boolean))];
    return '<details class="argus-detail" data-source-demo'+(D.ui.sourceDemoOpen?' open':'')+'><summary>'+e(D.t('sourceSimulation'))+'</summary><p>'+e(D.t('sourceHistoryHint'))+'</p><div class="argus-stack">'+ids.map(id=>{
      const state=D.state.sourceLifecycle?.[id]||{generation:1};
      return '<div class="argus-row"><strong>'+e(D.sources.providers[id]?.name||id)+'</strong>'+V().badge(D.t(state.stopped?'sourceStopped':'sourceRunning'))+V().badge(D.t('sourceGeneration',{n:state.generation||1}))+
        '<button class="argus-button argus-button--small" data-feature="source-stop" data-provider="'+id+'">'+e(D.t('stopSource'))+'</button><button class="argus-button argus-button--small" data-feature="source-restart" data-provider="'+id+'">'+e(D.t('restartSource'))+'</button></div>';
    }).join('')+'<button class="argus-button" data-feature="source-reset">'+e(D.t('sourceResetDemo'))+'</button></div></details>';
  }
  function isModal(kind){return ['lifecycle','lifecycle-preview'].includes(kind)}
  function modal(u){
    const v=V();
    if(u.kind==='lifecycle-preview'){
      const p=u.plan;
      return v.modal(D.t('lifecyclePreview'),p.error?v.notice(D.t(p.error),'warning'):'<h3>'+e(p.label)+'</h3><p>'+e(p.kind==='move'?D.t('moveUngrouped'):D.t(p.next==='archived'?'archive':'restore'))+'</p>'+v.notice(D.t('lifecycleHint')),
        v.button('cancel','modal-close')+(!p.error?'<button class="argus-button argus-button--primary" data-feature="lifecycle-commit">'+e(D.t('lifecycleConfirm'))+'</button>':''),false);
    }
    return v.modal(D.t('lifecycle'),v.notice(D.t('lifecycleHint'))+'<h3>'+e(D.t('boardState'))+'</h3><div class="argus-stack">'+D.state.dashboards.filter(b=>M.canRead(b.id)).map(b=>'<div class="argus-lifecycle-row"><span>'+e(M.label(b))+' '+v.badge(b.lifecycle==='archived'?D.t('archived'):D.t('published')+' v'+b.revision)+(M.getDraft(b.id)?v.badge(D.t('draft'),'warning'):'')+'</span><div class="argus-row"><button class="argus-button argus-button--small" data-feature="lifecycle-preview" data-kind="dashboard" data-id="'+b.id+'">'+e(D.t(b.lifecycle==='archived'?'restore':'archive'))+'</button>'+(b.folder?'<button class="argus-button argus-button--small" data-feature="move" data-id="'+b.id+'">'+e(D.t('moveUngrouped'))+'</button>':'')+'</div></div>').join('')+'</div><h3>'+e(D.t('folderState'))+'</h3>'+D.state.folders.map(f=>'<div class="argus-lifecycle-row"><span>'+e(M.label(f))+' · '+D.state.dashboards.filter(b=>b.folder===f.id).length+'</span><button class="argus-button" data-feature="lifecycle-preview" data-kind="folder" data-id="'+f.id+'">'+e(D.t(f.status==='archived'?'restore':'archive'))+'</button></div>').join(''),v.button('close','modal-close'));
  }
  function tryChange(id){
    if(!['correct-config','apm-drilldowns','latest-chat','source-history','archive-recovery'].includes(id))return false;
    if(id==='latest-chat'){D.navigate('chat');D.ui.chatChoice=['payment','apm-review'];D.openModal({kind:'chat-picker'});return true}
    if(!M.actor().manage&&id!=='apm-drilldowns'){D.state.actor='editor';M.persist()}
    D.openBoard('apm-review');
    if(id==='correct-config')D.editBoard('apm-review');
    if(id==='archive-recovery')D.openModal({kind:'lifecycle'});
    return true;
  }
  document.addEventListener('toggle',event=>{if(event.target.hasAttribute?.('data-source-demo'))D.ui.sourceDemoOpen=event.target.open},true);
  document.addEventListener('click',event=>{
    const el=event.target.closest('[data-feature],[data-action="feature-open-apm"],[data-action="feature-fullscreen"]');if(!el||(el.closest('#app')&&D.ui.modal))return;
    const a=el.dataset.feature||el.dataset.action;
    if(a==='feature-open-apm'){D.openBoard('apm-review');return}
    if(a==='feature-fullscreen'){D.ui.focusMode=!D.ui.focusMode;D.render();return}
    if(!M.actor().manage)return;
    if(a==='lifecycle'){D.openModal({kind:'lifecycle'});return}
    if(a==='lifecycle-preview'){D.openModal({kind:'lifecycle-preview',plan:D.workflow.archivePlan(el.dataset.kind,el.dataset.id)});return}
    if(a==='move'){const b=M.board(el.dataset.id);D.openModal({kind:'lifecycle-preview',plan:{kind:'move',id:b.id,expected:b.objectVersion||0,label:M.label(b)}});return}
    if(a==='lifecycle-commit'){
      const result=D.workflow.lifecycleCommit(D.ui.modal.plan);
      if(result.error){D.ui.modal.plan.error=result.error;D.renderModal(false);return}
      D.ui.view='dashboards';D.openModal({kind:'lifecycle'});D.render();D.toast(D.t('lifecycleDone'));return;
    }
    if(a==='rebase'){
      const b=M.board(D.ui.dash),d=M.getDraft(D.ui.dash);if(!b||b.lifecycle==='archived'||!d)return;
      d.base=b.revision;d.baseObjectVersion=b.objectVersion||0;M.save(d);D.openModal({kind:'preview',preview:M.preview(d,D.ui.scene)});return;
    }
    if(a==='next-version'){
      const b=M.board('apm-review');if(!b||b.lifecycle==='archived')return;
      b.revision++;b.updated=Date.now();M.persist();D.render();D.toast(D.t('nextVersionHint'));return;
    }
    if(a.startsWith('source-')){
      D.state.sourceLifecycle??={};
      if(a==='source-reset')D.state.sourceLifecycle={};
      else {const s=D.state.sourceLifecycle[el.dataset.provider]??={generation:1,stopped:false};if(a==='source-stop')s.stopped=true;else if(a==='source-restart'){s.stopped=false;s.generation++}}
      M.persist();D.ui.sourceNotices={};D.sourceViews.normalize(D.currentData(),D.ui.ctx,D.ui.scene);D.render();
    }
  });
  D.features={reviewLinks,dashboardTools,sourceControls,isModal,modal,tryChange};
})();
