(() => {
  'use strict';
  const D=window.ArgusDemo,M=D.model,V=D.views,t=D.t;
  D.locale=D.state.locale;
  D.visuals?.upgrade(D.state);D.sources?.upgrade(D.state);D.apm?.upgrade(D.state);D.grid.normalizeState(D.state);
  D.ui={view:'review',dash:'payment',ctx:M.context(M.board('payment')),scene:'normal',chatScene:'normal',search:'',folder:'all',modal:null,guide:null,notice:'',chatChoice:[],chatInput:'',chatBusy:false,chatStage:'stageQuery'};
  let refreshTimer=null,toastTimer=null,analysisToken=0,returnFocus=null;
  D.chat=()=>D.state.chat[D.state.actor];
  D.currentData=()=>D.ui.view==='edit'?M.getDraft(D.ui.dash)?.data:M.board(D.ui.dash);
  D.toast=message=>{
    const node=document.getElementById('toast');node.textContent=message;node.hidden=false;
    clearTimeout(toastTimer);toastTimer=setTimeout(()=>node.hidden=true,4500);
  };
  function render(){
    D.visuals?.dispose();D.visuals?.upgrade(D.state);D.sources?.upgrade(D.state);D.apm?.upgrade(D.state);D.grid.normalizeState(D.state);
    D.locale=D.state.locale;document.documentElement.lang=D.locale;document.documentElement.dataset.theme=D.state.theme;
    let content='';
    if(D.ui.view==='review')content=V.review();
    if(D.ui.view==='dashboards')content=V.catalog();
    if(D.ui.view==='dashboard')content=V.dashboard(false);
    if(D.ui.view==='edit')content=V.dashboard(true);
    if(D.ui.view==='hosts'||D.ui.view==='clusters')content=V.resourcePage(D.ui.view==='hosts'?'host':'cluster');
    if(D.ui.view==='chat')content=V.chat();
    document.getElementById('app').innerHTML=V.shell(content);
    renderModal(false);configureRefresh();D.visuals?.mount();
  }
  function refreshPanel(id){
    const data=D.currentData(),p=data?.panels.find(x=>x.id===id),node=document.querySelector('[data-panel-id="'+CSS.escape(id)+'"].argus-panel');
    if(!p||!node)return;
    D.visuals?.disposePanel(id);
    node.outerHTML=V.panelCard(p,data,D.ui.view==='edit');
    D.visuals?.mount(document.querySelector('[data-panel-id="'+CSS.escape(id)+'"].argus-panel'));
  }
  function renderModal(focus=true){
    const root=document.getElementById('modal-root');
    const previous=document.activeElement;
    const anchor=previous?.closest('#modal-root')?{field:previous.dataset.field,store:previous.dataset.store,action:previous.dataset.action,mode:previous.dataset.mode}:null;
    D.visuals?.disposeModal();root.innerHTML=V.dialog();
    const app=document.getElementById('app');app.inert=!!D.ui.modal;
    document.body.style.overflow=D.ui.modal?'hidden':'';
    if(focus&&D.ui.modal)root.querySelector('input:not([type=checkbox]),select,textarea,button')?.focus();
    else if(anchor&&D.ui.modal){
      let selector=anchor.field?'[data-field="'+CSS.escape(anchor.field)+'"][data-store="'+CSS.escape(anchor.store||'')+'"]':anchor.action?'[data-action="'+CSS.escape(anchor.action)+'"]':'';
      if(selector&&anchor.mode)selector+='[data-mode="'+CSS.escape(anchor.mode)+'"]';
      if(selector)root.querySelector(selector)?.focus({preventScroll:true});
    }
    D.visuals?.mountModal();
  }
  function openModal(modal){
    returnFocus=document.activeElement;
    D.ui.modal=modal;renderModal();
  }
  function closeModal(){
    D.ui.modal=null;renderModal(false);
    if(returnFocus?.isConnected)returnFocus.focus();
  }
  function refreshNotice(data){
    const changes=M.normalizeContext(D.ui.ctx,data.variables,D.ui.scene);
    if(changes.length)D.ui.notice=changes.map(c=>t('fallback',c)).join(' ');
    D.ui.sourceNotices={};D.sourceViews?.normalize(data,D.ui.ctx,D.ui.scene);
    return changes;
  }
  function openBoard(id,resource='all'){
    const data=M.board(id);if(!data||data.lifecycle==='archived'){D.ui.view='dashboards';render();D.toast(t(data?'unavailable':'unauthorized'));return}
    closeModal();D.ui.focusMode=false;D.ui.view='dashboard';D.ui.dash=id;D.ui.ctx=M.context(data);
    D.ui.sourcePending={};D.ui.sourceMore={};D.ui.vizFilters={};
    D.ui.ctx.resource=M.allowedResources().some(r=>r.id===resource)?resource:'all';D.ui.notice='';D.ui.guide=null;
    refreshNotice(data);render();window.scrollTo(0,0);
  }
  function editBoard(id){
    if(!M.actor().manage){D.toast(t('unauthorized'));return}
    if(M.board(id)?.lifecycle==='archived'){D.toast(t('unavailable'));return}
    const draft=M.getDraft(id)||M.beginDraft(id);if(!draft)return;
    closeModal();D.ui.focusMode=false;D.ui.view='edit';D.ui.dash=id;D.ui.ctx=M.context(draft.data);D.ui.notice='';D.ui.guide=null;
    D.ui.sourcePending={};D.ui.sourceMore={};D.ui.vizFilters={};
    refreshNotice(draft.data);render();window.scrollTo(0,0);
  }
  function navigate(view){
    closeModal();D.ui.focusMode=false;D.ui.view=view;D.ui.guide=null;D.ui.notice='';render();window.scrollTo(0,0);
  }
  function saveDraft(){
    const draft=M.getDraft(D.ui.dash);if(!draft||!M.actor().manage)return;
    M.save(draft);
    const node=document.getElementById('draft-save-state');
    if(node)node.textContent=t('savedAt',{time:V.stamp()});
  }
  function configureRefresh(){
    clearInterval(refreshTimer);
    if(D.ui.view!=='dashboard'||!D.ui.ctx.refresh)return;
    refreshTimer=setInterval(()=>{
      if(!document.hidden&&!D.ui.modal){D.ui.notice='';refreshNotice(D.currentData());render();D.toast(t('refreshed'))}
    },Number(D.ui.ctx.refresh)*1000);
  }
  function newPanel(){
    const draft=M.getDraft(D.ui.dash);if(!draft)return;
    const p=M.panel('panel-'+Date.now().toString(36),'metrics',D.local('新统计图','New panel'),D.local('新统计图','New panel'),'rate');
    p.builder.variable=draft.data.variables.some(v=>v.key==='service')?'service':'';
    p.builder.environmentVariable=draft.data.variables.some(v=>v.key==='environment')?'environment':'';
    D.sources?.selectProvider(p,'prometheus');
    openModal({kind:'panel',panel:p,isNew:true,validation:null,sample:false});
  }
  function showVariables(index=0){
    const draft=M.getDraft(D.ui.dash);if(!draft||!M.actor().manage)return;
    openModal({kind:'variables',variables:M.copy(draft.data.variables),index,error:null});
  }
  function preparePreview(id=D.ui.dash){
    const draft=M.getDraft(id);if(!draft)return;
    D.ui.dash=id;
    openModal({kind:'preview',preview:M.preview(draft,D.ui.scene),error:null});
  }
  function tryChange(id){
    if(D.features?.tryChange(id))return;
    if(D.sourceViews?.tryChange(id))return;
    if(D.visuals?.tryChange(id))return;
    if(id==='access'){D.state.actor='viewer';M.persist();openBoard('payment');return}
    if(!M.actor().manage){D.state.actor='editor';M.persist()}
    D.ui.scene='normal';
    if(id==='draft'){editBoard('payment');return}
    if(id==='fallback'){
      openBoard('payment');D.ui.ctx.range='1h';D.ui.ctx.vars.environment=['staging'];D.ui.ctx.vars.service=['All'];D.ui.guide='fallback';D.ui.notice='';render();return;
    }
    if(id==='variables'){editBoard('payment');showVariables();return}
    if(id==='panel'){editBoard('payment');openModal({kind:'panel',panel:M.copy(M.getDraft('payment').data.panels[0]),isNew:false});return}
    if(id==='resource'){navigate('hosts');return}
    if(id==='catalog'){openBoard('payment');openModal({kind:'catalog',signal:'metrics',source:'all'});return}
    if(id==='chat'){navigate('chat');return}
  }
  function selectedChatBoards(){return D.chat().selected.map(M.board).filter(Boolean)}
  function parseContext(question,boards){
    const result=D.workflow.plan(boards,D.chat().context,question,D.ui.chatScene);
    if(!result.error&&!result.clarification)D.chat().context=result.context;
    return result;
  }
  const delay=ms=>new Promise(resolve=>setTimeout(resolve,ms));
  async function analyze(question){
    const chat=D.chat(),chosen=selectedChatBoards().map(M.copy),queryScene=D.ui.chatScene;if(!chosen.length)return;
    const normalized=parseContext(question,chosen);
    if(normalized?.error){chat.messages.push({kind:'text',key:normalized.error});M.persist();render();return}
    if(normalized?.clarification){chat.messages.push({kind:'text',text:normalized.clarification});M.persist();render();return}
    const {entries,inherited,fallbacks,notices}=normalized;
    notices.forEach(text=>chat.messages.push({kind:'text',text}));
    if(fallbacks.length)chat.messages.push({kind:'text',text:fallbacks.map(c=>t('fallback',c)).join(' ')});
    chat.messages.push({kind:'text',key:inherited?'inherited':'newContext'});
    const token=++analysisToken,owner=D.state.actor;
    D.ui.chatBusy=true;D.ui.chatStage='stageQuery';M.persist();render();
    await delay(400);if(token!==analysisToken||owner!==D.state.actor)return;
    D.ui.chatStage='stageFiles';render();
    const snapshotEnd=Date.now(),executionId='demo-'+Date.now().toString(36);
    const files=chosen.flatMap(b=>{const context={...entries[b.id].ctx,snapshotEnd,executionId},base=M.files(b,context,question,queryScene);return base.concat(D.workflow.detailFiles(b,context,question,queryScene,base))});
    await delay(450);if(token!==analysisToken||owner!==D.state.actor)return;
    D.ui.chatStage='stageAnalyze';render();
    await delay(450);if(token!==analysisToken||owner!==D.state.actor)return;
    const scope=chosen.map(b=>{const ctx=entries[b.id].ctx;return M.label(b)+' v'+b.revision+' / '+t('range.'+ctx.range)+' / '+(ctx.resource==='all'?t('allResources'):M.resources.find(r=>r.id===ctx.resource)?.name)}).join(' · ');
    chat.messages.push({kind:'analysis',files,scope});D.ui.chatBusy=false;M.persist();render();
  }
  async function sendChat(question){
    if(D.ui.chatBusy)return;
    question=String(question||'').trim();if(!question)return;
    D.ui.chatInput='';
    const chat=D.chat();chat.messages.push({kind:'user',text:question});
    if(/^\/(创建仪表盘|Create dashboard)/i.test(question)){
      if(!M.actor().manage){chat.messages.push({kind:'text',key:'unauthorized'});M.persist();render();return}
      M.persist();render();openModal({kind:'ai-create',name:D.local('AI 支付巡检','AI payment review')});return;
    }
    if(question.includes('@')){
      const matches=M.boards().filter(b=>question.includes('@'+M.label(b))||question.includes('@'+b.id)||question.includes('@'+b.name));
      if(matches.length){chat.selected=matches.map(b=>b.id);chat.context=null}
    }
    chat.selected=chat.selected.filter(M.canRead);
    if(!chat.selected.length){
      D.ui.pendingQuestion=question;D.ui.chatChoice=[];chat.messages.push({kind:'choice'});M.persist();render();return;
    }
    await analyze(question);
  }
  const actions={
    nav:el=>navigate(el.dataset.view),
    theme:()=>{D.state.theme=D.state.theme==='dark'?'light':'dark';M.persist();render()},
    locale:()=>{D.state.locale=D.state.locale==='zh-CN'?'en-US':'zh-CN';M.persist();render()},
    try:el=>tryChange(el.dataset.id),
    open:el=>openBoard(el.dataset.id),
    edit:el=>editBoard(el.dataset.id),
    'view-published':()=>{const b=M.board(D.ui.dash);if(b)openBoard(b.id);else navigate('dashboards')},
    'new-dashboard':()=>openModal({kind:'create',name:'',description:'',folder:D.ui.folder==='all'?'none':D.ui.folder}),
    'create-draft':()=>{
      const u=D.ui.modal;if(!u.name.trim()){D.toast(t('name'));return}
      const d=M.createDraft(u.name.trim(),u.description,u.folder==='none'?null:u.folder);editBoard(d.id);
    },
    'new-folder':()=>openModal({kind:'folder',name:''}),
    'create-folder':()=>{const name=D.ui.modal.name.trim();if(!name)return;D.state.folders.push({id:'folder-'+Date.now(),name,nameEn:name});M.persist();closeModal();render()},
    'save-draft':()=>{saveDraft();D.toast(t('saved'))},
    discard:()=>{if(!M.actor().manage)return;delete D.state.drafts[M.draftKey(D.ui.dash)];M.persist();if(M.board(D.ui.dash))openBoard(D.ui.dash);else navigate('dashboards')},
    preview:()=>preparePreview(),
    'preview-ai':el=>preparePreview(el.dataset.id),
    publish:()=>{
      const u=D.ui.modal;if(u?.kind!=='preview')return;
      const result=M.publish(u.preview);
      if(result.error){
        if(result.error==='conflict'){openModal({kind:'conflict',resolution:'published'});return}
        u.error=result.error;renderModal();return;
      }
      const id=result.data.id,rev=result.data.revision;
      closeModal();
      if(result.origin==='ai'){
        D.chat().messages.push({kind:'published',id,revision:rev});M.persist();navigate('chat');
      }else openBoard(id);
      D.toast(t('publishSuccess',{n:rev}));
    },
    'simulate-conflict':()=>{M.conflict(D.ui.modal.preview.id);D.ui.modal.error='conflict';renderModal()},
    'resolve-conflict':()=>{
      const u=D.ui.modal,d=M.getDraft(D.ui.dash),current=M.board(D.ui.dash);
      if(!d||!current)return;
      if(u.resolution!=='mine'){d.data.description=current.description;d.data.descriptionEn=current.descriptionEn}
      d.base=current.revision;d.baseObjectVersion=current.objectVersion||0;M.save(d);closeModal();editBoard(d.id);D.toast(t('needPreview'));
    },
    'modal-close':()=>closeModal(),
    variables:()=>showVariables(),
    'variable-select':el=>{D.ui.modal.index=Number(el.dataset.index);D.ui.modal.error=null;renderModal(false)},
    'variable-add':()=>{
      const u=D.ui.modal;
      let n=u.variables.length+1;while(u.variables.some(v=>v.key==='variable'+n))n++;
      u.variables.push({key:'variable'+n,label:D.local('新变量','New variable'),labelEn:'New variable',signal:'metrics',item:'http_requests_total',field:'service',dependency:'',selection:'single',defaultValues:['All'],refresh:'data'});
      u.index=u.variables.length-1;renderModal(false);
    },
    'variable-remove':()=>{
      const u=D.ui.modal,v=u.variables[u.index],data=D.currentData();
      if(data.panels.some(p=>M.uses(p,v.key))||u.variables.some(x=>x.dependency===v.key)){u.error='referenced';renderModal(false);return}
      u.variables.splice(u.index,1);u.index=Math.max(0,u.index-1);renderModal(false);
    },
    'variables-save':()=>{
      const u=D.ui.modal;if(M.variableErrors(u.variables).length){u.error='variableInvalid';renderModal();return}
      const d=M.getDraft(D.ui.dash);if(!d)return;d.data.variables=M.copy(u.variables);M.save(d);
      closeModal();refreshNotice(d.data);render();D.toast(t('saved'));
    },
    'panel-new':()=>newPanel(),
    'panel-edit':el=>{
      const p=M.getDraft(D.ui.dash)?.data.panels.find(x=>x.id===el.dataset.id);if(p)openModal({kind:'panel',panel:M.copy(p),isNew:false});
    },
    'panel-mode':el=>{
      const p=D.ui.modal.panel;if(!M.setMode(p,el.dataset.mode)){D.toast(t('cantConvert'));return}
      D.ui.modal.validation=null;renderModal(false);
    },
    'panel-save':()=>{
      const u=D.ui.modal,d=M.getDraft(D.ui.dash);if(!d||!M.actor().manage)return;
      const p=M.copy(u.panel);p.expression=M.expression(p);if(p.drilldowns===undefined&&D.apm)p.drilldowns=D.apm.standardDrilldowns(p);
      if(u.isNew)d.data.panels.push(p);else d.data.panels[d.data.panels.findIndex(x=>x.id===p.id)]=p;
      D.grid.ensure(d.data.panels);
      D.grid.resolve(d.data.panels,p.id,p.layout).forEach(item=>{const target=d.data.panels.find(x=>x.id===item.id);target.layout=item.layout;target.width=item.layout.w});
      if(D.ui.ctx.localFilters)delete D.ui.ctx.localFilters[p.id];
      if(D.ui.sourcePending)delete D.ui.sourcePending[p.id];
      M.save(d);closeModal();refreshNotice(d.data);render();D.toast(t('saved'));
    },
    'panel-remove':()=>{
      const u=D.ui.modal,d=M.getDraft(D.ui.dash);d.data.panels=d.data.panels.filter(p=>p.id!==u.panel.id);M.save(d);closeModal();render();
    },
    'validate-panel':()=>{D.ui.modal.validation=M.validExpression(D.ui.modal.panel)?'simValidation':'invalidQuery';renderModal(false)},
    'sample-panel':()=>{D.ui.modal.validation=M.validExpression(D.ui.modal.panel)?'simValidation':'invalidQuery';D.ui.modal.sample=D.ui.modal.validation==='simValidation';renderModal(false)},
    'query-read':el=>{const p=D.currentData()?.panels.find(x=>x.id===el.dataset.id);if(p)openModal({kind:'query',panel:p})},
    'data-catalog':()=>openModal({kind:'catalog',signal:'metrics',source:'all'}),
    'catalog-signal':el=>{D.ui.modal.signal=el.dataset.signal;D.ui.modal.source='all';renderModal(false)},
    'catalog-use':el=>{
      const signal=el.dataset.signal,item=el.dataset.item,field=el.dataset.fieldName,d=M.getDraft(D.ui.dash);if(!d)return;
      showVariables();const u=D.ui.modal;let key=field.replace(/\W/g,'_');while(u.variables.some(v=>v.key===key))key+='_new';
      u.variables.push({key,label:field,labelEn:field,signal,item,field,dependency:'',selection:'single',defaultValues:['All'],refresh:'data'});u.index=u.variables.length-1;renderModal(false);
    },
    refresh:()=>{D.ui.notice='';refreshNotice(D.currentData());render();D.toast(t('refreshed'))},
    switch15:()=>{D.ui.ctx.range='15m';D.ui.notice='';refreshNotice(D.currentData());render()},
    'time-apply':()=>{
      const u=D.ui.modal;if(!u.from||!u.to||Date.parse(u.to)<=Date.parse(u.from)){D.toast(t('time'));return}
      D.ui.ctx.range='custom';D.ui.ctx.from=u.from;D.ui.ctx.to=u.to;closeModal();refreshNotice(D.currentData());render();
    },
    bindings:el=>openModal({kind:'bindings',resource:el.dataset.id,ids:[...(D.state.bindings[el.dataset.id]||[])]}),
    'binding-preview':()=>{const u=D.ui.modal;openModal({kind:'binding-preview',resource:u.resource,ids:[...u.ids]})},
    'binding-confirm':()=>{const u=D.ui.modal;if(!M.actor().manage)return;D.state.bindings[u.resource]=u.ids;M.persist();closeModal();render();D.toast(t('linked'))},
    'resource-open':el=>openBoard(el.dataset.id,el.dataset.resource),
    'chat-picker':()=>{D.ui.chatChoice=[...D.chat().selected];openModal({kind:'chat-picker'})},
    'chat-choice':async()=>{
      const selected=D.ui.chatChoice.filter(M.canRead);if(!selected.length)return;
      D.chat().selected=[...selected];D.chat().context=null;
      D.chat().messages=D.chat().messages.filter(m=>m.kind!=='choice');
      closeModal();M.persist();render();
      if(D.ui.pendingQuestion){const q=D.ui.pendingQuestion;D.ui.pendingQuestion='';await analyze(q)}
    },
    'chat-send':()=>sendChat(document.getElementById('chat-input')?.value),
    'chat-quick':el=>sendChat(el.dataset.question),
    'chat-clear':()=>{analysisToken++;D.ui.chatBusy=false;D.ui.pendingQuestion='';D.state.chat[D.state.actor]={messages:[],context:null,selected:[]};M.persist();render()},
    'ai-create':()=>{if(!M.actor().manage){D.toast(t('unauthorized'));return}openModal({kind:'ai-create',name:D.local('AI 支付巡检','AI payment review')})},
    'ai-generate':el=>{
      if(!M.actor().manage)return;
      const name=D.ui.modal.name.trim();if(!name)return;
      const d=M.createDraft(name,D.local('通过 Chat 生成的模拟草稿。','Simulated draft generated through Chat.'),'prod','ai',el.dataset.mode);
      D.chat().messages.push({kind:'created',id:d.id,name});M.persist();closeModal();navigate('chat');
    },
    trace:el=>{
      const data=D.currentData();const record=data?.panels.flatMap(p=>M.panelData(p,D.ui.ctx,data.variables,D.ui.scene).rows).find(r=>r.id===el.dataset.id);if(record)openModal({kind:'trace',record});
    },
    reset:()=>openModal({kind:'reset'}),
    'reset-confirm':()=>{analysisToken++;D.state=M.seed();M.persist();D.ui.chatBusy=false;D.ui.modal=null;D.ui.scene='normal';D.ui.chatScene='normal';D.ui.ctx=M.context(M.board('payment'));navigate('review')}
  };
  document.addEventListener('click',event=>{
    const el=event.target.closest('[data-action]');if(!el||el.disabled)return;
    if(el.closest('#app')&&D.ui.modal)return;
    const fn=actions[el.dataset.action];if(fn){event.preventDefault();Promise.resolve(fn(el)).catch(err=>{console.error(err);D.toast(D.local('演示操作失败，请刷新后重试。','Demo action failed. Reload and retry.'))})}
  });
  document.addEventListener('input',event=>{
    const el=event.target;
    if(el.dataset.note){D.state.notes[el.dataset.note]=el.value;M.persist();return}
    if(el.id==='chat-input'){D.ui.chatInput=el.value;return}
    if(el.id==='dashboard-search'){
      D.ui.search=el.value;const pos=el.selectionStart;render();const next=document.getElementById('dashboard-search');next.focus();next.setSelectionRange(pos,pos);return;
    }
    const field=el.dataset.field,store=el.dataset.store;if(!field)return;
    if(store==='draft'&&['name','description'].includes(field)){
      const d=M.getDraft(D.ui.dash);d.data[field]=el.value;d.data[field+'En']=el.value;saveDraft();return;
    }
    if(store==='panel'&&['title','description','expression','unit'].includes(field)){
      const p=D.ui.modal.panel;p[field]=el.value;if(field==='title'||field==='description')p[field+'En']=el.value;
      D.ui.modal.validation=null;
      const toBuilder=document.getElementById('to-builder');if(toBuilder)toBuilder.disabled=!M.canBuild(p);
      const conversionNote=document.getElementById('conversion-note');if(conversionNote)conversionNote.hidden=M.canBuild(p);
      return;
    }
    if(['create','folder','time','ai-create'].includes(store)){D.ui.modal[field]=el.value;return}
    if(store==='variable'&&['key','label'].includes(field)){
      const v=D.ui.modal.variables[D.ui.modal.index];v[field]=el.value;if(field==='label')v.labelEn=el.value;
    }
  });
  document.addEventListener('change',event=>{
    const el=event.target;
    if(el.dataset.setting==='actor'){
      analysisToken++;D.ui.chatBusy=false;D.state.actor=el.value;M.persist();D.ui.modal=null;
      if(['edit','dashboard'].includes(D.ui.view))openBoard(M.board(D.ui.dash)?D.ui.dash:'payment');else render();return;
    }
    if(el.dataset.review){D.state.review[el.dataset.review]=el.checked;M.persist();render();return}
    if(el.dataset.chatChoice){const ids=new Set(D.ui.chatChoice);el.checked?ids.add(el.dataset.chatChoice):ids.delete(el.dataset.chatChoice);D.ui.chatChoice=[...ids];return}
    if(el.dataset.binding){const ids=new Set(D.ui.modal.ids);el.checked?ids.add(el.dataset.binding):ids.delete(el.dataset.binding);D.ui.modal.ids=[...ids];return}
    if(el.hasAttribute('data-resolve')){D.ui.modal.resolution=el.value;return}
    if(el.dataset.filter){
      const key=el.dataset.filter,value=el.dataset.value,v=D.currentData().variables.find(x=>x.key===key);
      let values=D.ui.ctx.vars[key]||['All'];
      if(value==='All'||v.selection==='single')values=[value];
      else {values=values.filter(x=>x!=='All'&&x!==value);if(el.checked)values.push(value);if(!values.length)values=['All']}
      D.ui.ctx.vars[key]=values;D.ui.notice='';refreshNotice(D.currentData());render();return;
    }
    const field=el.dataset.field,store=el.dataset.store;if(!field)return;
    if(store==='catalog'){D.ui.folder=el.value;render();return}
    if(store==='chat-scene'){D.ui.chatScene=el.value;render();return}
    if(store==='scene'){D.ui.scene=el.value;D.ui.notice='';refreshNotice(D.currentData());render();return}
    if(store==='context'){
      if(field==='range'&&el.value==='custom'){
        const now=new Date(),before=new Date(now.getTime()-3600000),local=d=>new Date(d.getTime()-d.getTimezoneOffset()*60000).toISOString().slice(0,16);
        openModal({kind:'time',from:D.ui.ctx.from||local(before),to:D.ui.ctx.to||local(now)});return;
      }
      D.ui.ctx[field]=field==='refresh'?Number(el.value):el.value;D.ui.notice='';refreshNotice(D.currentData());render();return;
    }
    if(store==='draft'){
      const d=M.getDraft(D.ui.dash);d.data[field]=field==='defaultRefresh'?Number(el.value):field==='folder'&&el.value==='none'?null:el.value;saveDraft();return;
    }
    if(store==='panel'){
      const p=D.ui.modal.panel;
      if(field==='signal'){
        p.signal=el.value;p.builder=M.builder(p.signal);
        const vars=D.currentData().variables;p.builder.variable=vars.some(v=>v.key==='service')?'service':'';p.builder.environmentVariable=vars.some(v=>v.key==='environment')?'environment':'';
        p.mode='builder';p.type=M.types[p.signal][0];p.roundTrip=null;p.expression=M.expression(p);
        D.sources?.selectProvider(p,{metrics:'prometheus',logs:'otlp_logs',traces:'otel'}[p.signal]);
        if(D.apm)p.drilldowns=D.apm.standardDrilldowns(p);
      }else if(field==='type'&&el.value==='trace_detail'&&p.mode==='builder'){p.type=el.value;p.builder.op='detail';p.expression=M.expression(p)}else if(field==='resources')p.resources=el.value==='both'?['host','cluster']:[el.value];
      else p[field]=el.value;
      renderModal(false);return;
    }
    if(store==='panel-builder'){
      const p=D.ui.modal.panel;p.builder[field]=['limit','duration'].includes(field)?Number(el.value):el.value;
      if(field==='op'&&el.value==='p95')p.builder.metric='http_request_duration_seconds_bucket';
      if(!M.allowedTypes(p).includes(p.type))p.type=M.allowedTypes(p)[0];
      p.expression=M.expression(p);D.ui.modal.validation=null;renderModal(false);return;
    }
    if(store==='variable'){
      const u=D.ui.modal,v=u.variables[u.index];
      if(field==='defaultValues')v.defaultValues=[el.value];
      else v[field]=el.value;
      if(field==='signal'){v.item=M.catalog[v.signal][0].name;v.field=M.catalog[v.signal][0].fields[0]}
      if(field==='item')v.field=M.catalog[v.signal].find(x=>x.name===v.item).fields[0];
      u.error=null;renderModal(false);return;
    }
    if(store==='catalog-modal'){D.ui.modal.source=el.value;renderModal(false)}
  });
  document.addEventListener('keydown',event=>{
    if(event.key==='Escape'&&D.ui.modal){event.preventDefault();closeModal();return}
    if(event.key==='Enter'&&(event.ctrlKey||event.metaKey)&&event.target.id==='chat-input'){event.preventDefault();sendChat(event.target.value)}
    if(event.key==='Tab'&&D.ui.modal){
      const nodes=[...document.querySelectorAll('#modal-root button:not(:disabled),#modal-root input,#modal-root select,#modal-root textarea,#modal-root summary')].filter(n=>n.getClientRects().length);
      if(!nodes.length)return;
      const first=nodes[0],last=nodes.at(-1);
      if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus()}
      else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus()}
    }
  });
  window.addEventListener('storage',event=>{if(event.key===M.KEY){D.toast(D.local('另一页面修改了原型数据，请刷新以读取最新状态。','Another tab changed the prototype. Reload to read its latest state.'))}});
  D.render=render;D.refreshPanel=refreshPanel;D.openModal=openModal;D.renderModal=renderModal;D.closeModal=closeModal;D.openBoard=openBoard;D.editBoard=editBoard;D.navigate=navigate;
  window.addEventListener('hashchange',()=>{if(location.hash==='#sources')openBoard('source-workbench');else if(location.hash==='#apm')openBoard('apm-review')});
  if(location.hash==='#sources')openBoard('source-workbench');else if(location.hash==='#apm')openBoard('apm-review');else render();
})();
