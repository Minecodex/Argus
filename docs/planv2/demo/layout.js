(() => {
  'use strict';
  const D=window.ArgusDemo,M=D.model;
  const COLUMNS=12,ROW=32,GAP=16,MIN_W=3,MIN_H=5,MAX_H=28;
  const integer=(n,fallback)=>Number.isFinite(Number(n))?Math.round(Number(n)):fallback;
  const clamp=(n,min,max)=>Math.max(min,Math.min(max,n));
  function normalize(rect={}){
    const w=clamp(integer(rect.w,6),MIN_W,COLUMNS),h=clamp(integer(rect.h,8),MIN_H,MAX_H);
    return {x:clamp(integer(rect.x,0),0,COLUMNS-w),y:Math.max(0,integer(rect.y,0)),w,h};
  }
  function intersects(a,b){return a.x<b.x+b.w&&a.x+a.w>b.x&&a.y<b.y+b.h&&a.y+a.h>b.y}
  function ensure(panels){
    const placed=[];
    panels.forEach(p=>{
      if(!p.layout){
        const w=p.width===12?12:6;let candidate=null;
        for(let y=0;!candidate&&y<2000;y++)for(let x=0;x<=COLUMNS-w;x+=MIN_W){const r={x,y,w,h:p.signal==='logs'||p.signal==='traces'?10:8};if(!placed.some(q=>intersects(r,q))){candidate=r;break}}
        p.layout=candidate||{x:0,y:placed.reduce((v,r)=>Math.max(v,r.y+r.h),0),w,h:8};
      }else p.layout=normalize(p.layout);
      placed.push(p.layout);
    });
    return panels;
  }
  function resolve(panels,id,rect){
    const result=panels.map(p=>({id:p.id,layout:normalize(p.layout)}));
    const moving=result.find(p=>p.id===id);if(!moving)return result;
    moving.layout=normalize(rect);
    const done=[moving];
    const others=result.filter(p=>p!==moving).sort((a,b)=>a.layout.y-b.layout.y||a.layout.x-b.layout.x);
    for(const p of others){
      let conflict;
      while((conflict=done.find(q=>intersects(q.layout,p.layout))))p.layout.y=conflict.layout.y+conflict.layout.h;
      done.push(p);
    }
    return result;
  }
  function height(panels,editing=false){return (Math.max(1,...panels.map(p=>p.layout.y+p.layout.h))+(editing?3:0))*(ROW+GAP)-GAP}
  function style(rect){
    const r=normalize(rect);
    return 'left:calc('+r.x+' * (100% + var(--layout-gap)) / 12);top:'+(r.y*(ROW+GAP))+'px;width:calc('+r.w+' * (100% + var(--layout-gap)) / 12 - var(--layout-gap));height:'+(r.h*(ROW+GAP)-GAP)+'px;';
  }
  function commit(id,rect){
    if(D.ui?.view!=='edit'||!M.actor().manage)return;
    const d=M.getDraft(D.ui.dash);if(!d)return;
    ensure(d.data.panels);
    const changed=resolve(d.data.panels,id,rect);
    changed.forEach(r=>{const p=d.data.panels.find(p=>p.id===r.id);p.layout=r.layout;p.width=r.layout.w});
    M.save(d);D.render();
  }
  let active=null;
  function applyPreview(result){
    if(!active)return;
    const grid=active.grid;
    result.forEach(p=>{
      const card=[...grid.querySelectorAll('[data-panel-id]')].find(el=>el.dataset.panelId===p.id);
      if(card)card.style.cssText=style(p.layout);
    });
    grid.style.height=height(result.map(p=>({layout:p.layout})),true)+'px';
    const label=grid.querySelector('.argus-layout-status');
    if(label){const p=result.find(p=>p.id===active.id);label.textContent='x '+p.layout.x+' · y '+p.layout.y+' · '+p.layout.w+' × '+p.layout.h}
  }
  function finish(cancel=false){
    if(!active)return;
    const current=active;active=null;
    if(current.handle.hasPointerCapture?.(current.pointerId))current.handle.releasePointerCapture(current.pointerId);
    document.body.classList.remove('argus-layout-dragging');
    if(cancel){D.render();return}
    commit(current.id,current.next);
  }
  document.addEventListener('pointerdown',event=>{
    const handle=event.target.closest('[data-grid-handle]');
    if(!handle||event.button!==0||D.ui?.view!=='edit'||!M.actor().manage)return;
    const grid=handle.closest('[data-dashboard-grid]'),d=M.getDraft(D.ui.dash);
    if(!grid||!d)return;
    ensure(d.data.panels);
    const p=d.data.panels.find(p=>p.id===handle.dataset.panelId);if(!p)return;
    event.preventDefault();
    const box=grid.getBoundingClientRect();
    active={id:p.id,mode:handle.dataset.gridHandle,start:M.copy(p.layout),next:M.copy(p.layout),panels:M.copy(d.data.panels),grid,handle,pointerId:event.pointerId,x:event.clientX,y:event.clientY,scrollY:window.scrollY,cell:(box.width+GAP)/COLUMNS};
    handle.setPointerCapture(event.pointerId);
    document.body.classList.add('argus-layout-dragging');
    grid.querySelector('[data-panel-id="'+p.id+'"]').classList.add('is-layout-active');
  });
  document.addEventListener('pointermove',event=>{
    if(!active||event.pointerId!==active.pointerId)return;
    event.preventDefault();
    const dx=Math.round((event.clientX-active.x)/active.cell),dy=Math.round((event.clientY-active.y+window.scrollY-active.scrollY)/(ROW+GAP));
    active.next=active.mode==='resize'?normalize({...active.start,w:Math.min(COLUMNS-active.start.x,active.start.w+dx),h:active.start.h+dy}):normalize({...active.start,x:active.start.x+dx,y:active.start.y+dy});
    applyPreview(resolve(active.panels,active.id,active.next));
    if(event.clientY>window.innerHeight-48)window.scrollBy(0,16);
    else if(event.clientY<80)window.scrollBy(0,-16);
  });
  document.addEventListener('pointerup',event=>{if(active&&event.pointerId===active.pointerId)finish(false)});
  document.addEventListener('pointercancel',()=>finish(true));
  document.addEventListener('keydown',event=>{
    if(active&&event.key==='Escape'){event.preventDefault();finish(true);return}
    const handle=event.target.closest('[data-grid-handle]');if(!handle||!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(event.key))return;
    if(D.ui?.view!=='edit'||!M.actor().manage)return;
    event.preventDefault();
    const d=M.getDraft(D.ui.dash),p=d?.data.panels.find(p=>p.id===handle.dataset.panelId);if(!p)return;
    const r={...p.layout},step=event.shiftKey?2:1;
    const dx=event.key==='ArrowLeft'?-step:event.key==='ArrowRight'?step:0,dy=event.key==='ArrowUp'?-step:event.key==='ArrowDown'?step:0;
    if(handle.dataset.gridHandle==='resize'){r.w=Math.min(COLUMNS-r.x,r.w+dx);r.h+=dy}else{r.x+=dx;r.y+=dy}
    const mode=handle.dataset.gridHandle,id=p.id;commit(id,r);
    document.querySelector('[data-grid-handle="'+mode+'"][data-panel-id="'+id+'"]')?.focus({preventScroll:true});
  });
  function normalizeState(state){
    state.dashboards.forEach(b=>ensure(b.panels));
    Object.values(state.drafts).forEach(d=>ensure(d.data.panels));
  }
  D.grid={normalize,ensure,intersects,resolve,height,style,commit,normalizeState,columns:COLUMNS,row:ROW,gap:GAP};
})();
