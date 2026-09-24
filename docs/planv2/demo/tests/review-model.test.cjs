const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..');
function setup(){
  const storage=new Map();
  const box={console,Date,Math,JSON,Map,Set,setTimeout,clearTimeout,localStorage:{getItem:k=>storage.get(k)||null,setItem:(k,v)=>storage.set(k,v)},document:{addEventListener(){}}};
  box.window=box;vm.createContext(box);
  for(const file of ['i18n.js','model.js','layout.js','visualizations.js','source-data.js','source-views.js','workflow.js','apm.js','features.js','views.js'])vm.runInContext(fs.readFileSync(path.join(root,file),'utf8'),box,{filename:file});
  const D=box.ArgusDemo,M=D.model;D.locale='zh-CN';D.visuals.upgrade(D.state);D.sources.upgrade(D.state);D.apm.upgrade(D.state);
  const data=M.board('source-workbench');D.ui={view:'dashboard',dash:data.id,ctx:M.context(data),scene:'normal'};D.currentData=()=>M.board(D.ui.dash);
  return {D,M,S:D.sources,W:D.workflow,A:D.apm,data};
}
let count=0;
function test(name,fn){try{fn(setup());console.log('PASS '+name);count++}catch(err){console.error('FAIL '+name);throw err}}
test('UI-01 exact references ignore variable prefixes',({M,data})=>{
  const p=M.panel('x','metrics','x','x');p.builder.variable='service';assert.equal(M.uses(p,'serv'),false);assert(M.uses(p,'service'));
  const vars=[{key:'service',field:'service'},{key:'serv',field:'environment'}],ctx=M.context({...data,variables:[]});ctx.vars={service:['All'],serv:['staging']};
  assert(M.panelData(p,ctx,vars).rows.some(r=>r.environment==='production'));
});
test('UI-01 DSL tokens exclude comments and GraphQL native variables',({M})=>{
  const p=M.panel('x','metrics','x','x');M.setMode(p,'dsl');p.expression='sum(foo{service="$service",env="$'+'{environment}"}) # $comment';
  assert(!M.uses(p,'serv'));assert(M.uses(p,'service'));assert(!M.uses(p,'comment'));assert(M.uses(p,'environment'));
  p.signal='traces';p.expression='query($limit: Int) { queryTraces(pageSize: $limit, serviceName: "$service") { total } }';
  assert(!M.uses(p,'limit'));assert(M.uses(p,'service'));
});
test('UI-02 dangling DSL references block publication',({M})=>{
  const d=M.beginDraft('payment');d.data.panels.forEach(p=>M.setMode(p,'dsl'));d.data.variables.find(v=>v.key==='service').key='renamed_service';M.save(d);
  assert(M.validate(d.data).some(e=>e.includes('$service')));assert.equal(M.publish(M.preview(d,'normal')).error,'blocked');
});
test('UI-03 source change invalidates lossless conversion',({M,S,data})=>{
  const p=M.copy(data.panels.find(p=>p.signal==='metrics'));M.setMode(p,'dsl');assert(M.canBuild(p));S.selectProvider(p,'hostmetrics');
  assert(!M.canBuild(p));assert(!M.setMode(p,'builder'));assert(M.validate({...data,panels:[p]}).some(e=>e.includes('Host metrics')));
});
test('UI-03 compatible roundtrip keeps query',({M,data})=>{
  const p=M.copy(data.panels[0]),q=M.expression(p);M.setMode(p,'dsl');assert(M.setMode(p,'builder'));assert.equal(M.expression(p),q);
});
test('UI-04 log count renders a stat',({D,M,data})=>{
  const p=M.copy(data.panels[3]);p.builder.op='count';p.type='stat';const html=D.visuals.content(p,M.panelData(p,D.ui.ctx,data.variables));
  assert(html.includes('argus-stat'));assert(!html.includes('argus-log-line'));
});
test('UI-04 log trend and grouped table have distinct renderers',({D,M,data})=>{
  const p=M.copy(data.panels[3]);p.builder.op='trend';p.type='time_series';assert(D.visuals.content(p,M.panelData(p,D.ui.ctx,data.variables)).includes('data-echart'));
  p.builder.op='group_count';p.type='table';assert(D.visuals.content(p,M.panelData(p,D.ui.ctx,data.variables)).includes('payment-api'));
  p.type='time_series';assert(M.validate({...data,panels:[p]}).length>0);
});
test('UI-04 trace detail renders selector and spans',({D,M,data})=>{
  const p=M.copy(data.panels[0]);p.type='trace_detail';const html=D.visuals.content(p,M.panelData(p,D.ui.ctx,data.variables));assert(html.includes('data-trace-choice'));assert(html.includes('argus-trace-mini'));
});
test('UI-05 diff exposes query, defaults, refresh and layout',({M,W})=>{
  const a=M.copy(M.board('payment')),b=M.copy(a);b.panels[0].builder.window='15m';b.variables[0].defaultValues=['production'];b.defaultRefresh=300;b.panels[0].layout.w=9;
  const rows=W.diff(a,b);assert(rows.some(r=>r[2].includes('[15m]')));assert(rows.some(r=>r[2]==='production'));assert(rows.some(r=>r[2].includes('300')));assert(rows.some(r=>r[2].includes('9 ×')));
});
test('UI-06 independent defaults per dashboard',({M,W})=>{
  const a=M.board('payment'),b=M.board('apm-review'),p=W.plan([a,b],null,'有没有问题');assert.equal(p.entries[a.id].ctx.range,'1h');assert.equal(p.entries[b.id].ctx.range,'15m');
});
test('Q40 latest revision produces new files without modifying old ones',({M,W})=>{
  const b=M.board('apm-review'),first=W.plan([M.copy(b)],null,'有没有问题'),oldFiles=M.files(M.copy(b),first.entries[b.id].ctx,'有没有问题','normal'),text=JSON.stringify(oldFiles);
  b.revision++;b.defaultRange='6h';const next=W.plan([M.copy(b)],first.context,'有没有问题');assert.equal(next.entries[b.id].revision,b.revision);assert.equal(next.entries[b.id].ctx.range,'6h');assert(next.notices.length);assert.equal(JSON.stringify(oldFiles),text);
});
test('Q40 explicit compatible conditions survive revision updates',({M,W})=>{
  const b=M.board('source-workbench'),first=W.plan([b],null,'Jaeger 服务 payment-api 最近1小时');b.revision++;const next=W.plan([b],first.context,'那昨天呢');
  assert.equal(next.entries[b.id].ctx.range,'yesterday');assert.equal(next.entries[b.id].ctx.localFilters['source-jaeger'].service,'payment-api');
});
test('Q40 incompatible source conditions require clarification',({M,W})=>{
  const b=M.board('source-workbench'),first=W.plan([b],null,'Jaeger 服务 payment-api');b.panels.find(p=>p.id==='source-jaeger').sourceBinding='otel';b.revision++;assert(W.plan([b],first.context,'再看最近1小时').clarification);
});
test('Q37 stopping preserves history; reinstall separates generations',({D,M,S,data})=>{
  const p=data.panels[0],ctx=M.context(data),before=M.panelData(p,ctx,data.variables).rows;D.state.sourceLifecycle={skywalking:{generation:1,stopped:true}};const stopped=M.panelData(p,ctx,data.variables).rows;assert.equal(stopped.length,before.length);
  D.state.sourceLifecycle.skywalking={generation:2,stopped:false};const rows=M.panelData(p,ctx,data.variables).rows;assert(rows.some(r=>r.generation===1));assert(rows.some(r=>r.generation===2));assert(S.choices(p,'instance',ctx,data.variables).some(x=>x.includes('g2')));
  assert(before.every(r=>rows.some(x=>x.id===r.id&&x.sourceId===r.sourceId)));
});
test('Q37 execution freezes source state',({D,M,W,data})=>{
  const p=W.plan([data],null,'有没有问题'),ctx=p.entries[data.id].ctx,n=M.panelData(data.panels[0],ctx,data.variables).rows.length;D.state.sourceLifecycle={skywalking:{generation:2,stopped:true}};assert.equal(M.panelData(data.panels[0],ctx,data.variables).rows.length,n);
});
test('Q39 drilldown edits stay in drafts until publication',({M,A})=>{
  const b=M.board('apm-review'),d=M.beginDraft(b.id),p=d.data.panels[0];assert(A.allowed(p,'trace_detail'));p.drilldowns[0].enabled=false;M.save(d);assert(A.allowed(b.panels[0],'trace_detail'));
  const out=M.publish(M.preview(d,'normal'));assert(!out.error);assert(!A.allowed(out.data.panels[0],'trace_detail'));
});
test('Q39 drilldowns enforce target source and scope',({M,A})=>{
  const b=M.board('apm-review'),p=b.panels[0],ctx=M.context(b);ctx.resource='host-a';const r=M.panelData(p,ctx,b.variables).rows[0],out=A.traceTarget(p,r,ctx,'normal');assert(!out.error);assert.equal(out.record.resource,'host-a');
  p.drilldowns[0].enabled=false;assert.equal(A.traceTarget(p,r,ctx,'normal').error,'drilldownMissing');
});
test('Q42 archive retains draft and invalidates old preview',({M,W})=>{
  const d=M.beginDraft('payment'),preview=M.preview(d,'normal');assert(W.lifecycleCommit(W.archivePlan('dashboard','payment')).ok);assert(M.getDraft('payment'));assert.equal(M.publish(preview).error,'unavailable');
  assert(W.lifecycleCommit(W.archivePlan('dashboard','payment')).ok);assert.equal(M.publish(preview).error,'conflict');d.baseObjectVersion=M.board('payment').objectVersion;M.save(d);assert(!M.publish(M.preview(d,'normal')).error);
});
test('Q42 a nonempty folder cannot be archived',({M,W})=>{
  assert.equal(W.archivePlan('folder','infra').error,'folderNotEmpty');const b=M.board('infra');assert(W.lifecycleCommit({kind:'move',id:b.id,expected:b.objectVersion||0}).ok);assert(W.lifecycleCommit(W.archivePlan('folder','infra')).ok);
});
test('local filters cannot widen access or clear typed no-data conditions',({D,M,S,data})=>{
  const p=data.panels[0],ctx=M.context(data);ctx.resource='host-a';ctx.localFilters[p.id]={instance:'payment-api@host-b'};assert.equal(M.panelData(p,ctx,data.variables).rows.length,0);
  ctx.localFilters[p.id]={keyword:'no-such-record'};S.normalize(data,ctx);assert.equal(S.filterState(p,ctx).keyword,'no-such-record');assert.equal(M.panelData(p,ctx,data.variables).rows.length,0);
  D.state.actor='viewer';ctx.resource='host-b';assert.equal(M.panelData(p,ctx,data.variables).rows.length,0);
});
test('new defaults are not mistaken for explicit inherited filters',({M,W})=>{
  const b=M.board('source-workbench'),first=W.plan([b],null,'Jaeger 服务 payment-api');
  b.panels.find(p=>p.id==='source-jaeger').localDefaults.min=500;b.revision++;
  const next=W.plan([b],first.context,'有没有问题');
  assert.equal(next.entries[b.id].ctx.localFilters['source-jaeger'].min,500);
  assert(!Object.hasOwn(next.entries[b.id].explicit.locals['source-jaeger'],'min'));
});
test('UI and Chat execute the same published drilldown contract',({M,W,A})=>{
  const b=M.board('apm-review'),ctx=M.context(b);ctx.executionId='regression';ctx.resource='host-a';
  const p=b.panels[0],row=M.panelData(p,ctx,b.variables).rows.find(r=>r.traceId==='trace-01');
  const ui=A.resolve(p,'related_logs',row,ctx,b.variables,'normal'),files=W.detailFiles(b,ctx,'SkyWalking trace-01 关联日志','normal',M.files(b,ctx,'','normal'));
  assert.equal(files.length,1);assert.equal(files[0].count,ui.rows.length);assert.equal(files[0].drilldownRef,ui.definition.id);
  assert(ui.rows.every(r=>r.resource==='host-a'&&r.traceId==='trace-01'));
  p.drilldowns.find(d=>d.kind==='related_logs').enabled=false;
  assert.equal(W.detailFiles(b,ctx,'SkyWalking trace-01 关联日志','normal',[])[0].status,'unavailable');
});
test('aggregate inspection does not expose undeclared raw log results',({D,M,A})=>{
  const b=M.board('apm-review'),p=b.panels.find(p=>p.type==='stat'),ctx=M.context(b),result=M.panelData(p,ctx,b.variables);
  const html=D.visuals.content({...p,type:'table'},result);assert(!html.includes('risk provider timeout'));
  const raw=A.resolve(p,'log_records',null,ctx,b.variables,'normal');assert(raw.rows.length>0);assert(raw.rows.every(r=>r.severity==='ERROR'));
});
test('log trend stays inside its time window and preserves boundary counts',({M})=>{
  const b=M.board('apm-review'),p=b.panels.find(p=>p.type==='time_series'),ctx=M.context(b),rows=M.panelData(p,ctx,b.variables).rows,points=M.logAggregate(p,rows,ctx);
  assert.equal(points.reduce((n,p)=>n+p.count,0),rows.length);assert(points.every(p=>p.bucketMinutes<=M.duration(ctx)));
});
console.log('\n'+count+' regression cases passed.');
