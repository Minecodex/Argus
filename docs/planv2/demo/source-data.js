(() => {
  'use strict';
  const D=window.ArgusDemo,M=D.model;
  const providers={
    skywalking:{name:'SkyWalking',signal:'traces',receiver:'skywalking',secondary:'instance',detail:'endpoint'},
    jaeger:{name:'Jaeger',signal:'traces',receiver:'jaeger',secondary:'operation',detail:'tag'},
    otel:{name:'OpenTelemetry',signal:'traces',receiver:'otlp',secondary:'scope',detail:'kind'},
    filelog:{name:'File logs',signal:'logs',receiver:'filelog',secondary:'severity',detail:'file'},
    otlp_logs:{name:'OTLP Logs',signal:'logs',receiver:'otlp',secondary:'severity',detail:'scope'},
    prometheus:{name:'Prometheus',signal:'metrics',receiver:'prometheus',secondary:'instance',detail:'status'},
    hostmetrics:{name:'Host metrics',signal:'metrics',receiver:'hostmetrics',secondary:'instance',detail:'status'}
  };
  const defaults={service:'All',instance:'All',endpoint:'All',operation:'All',tag:'All',scope:'All',kind:'All',severity:'All',file:'All',status:'All',keyword:'',min:0};
  const fallback={traces:'otel',logs:'otlp_logs',metrics:'prometheus'};
  const provider=p=>providers[p.sourceBinding]||providers[fallback[p.signal]];
  const enabled=p=>!!providers[p.sourceBinding]&&provider(p).signal===p.signal;
  const definitions=p=>[...new Set(['service',provider(p).secondary,provider(p).detail,...(p.signal==='traces'?['status']:[])])];
  function filterState(p,ctx){
    const values={...defaults,...p.localDefaults,...ctx.localFilters?.[p.id]};
    const keys=[...definitions(p),'keyword',...(p.signal==='traces'?['min']:[])];
    return Object.fromEntries(keys.map(key=>[key,values[key]]));
  }
  function materialize(p,rows,ctx={}){
    if(!enabled(p))return rows;
    const lifecycle=(ctx.sourceSnapshot||D.state.sourceLifecycle||{})[p.sourceBinding]||{generation:1,stopped:false};
    const expanded=rows.map(r=>({...r,sourceGeneration:1}));
    const historical=ctx.range==='yesterday'||ctx.range==='custom'&&Date.parse(ctx.to)<Date.now()-60000;
    if(!historical)for(let generation=2;generation<=(lifecycle.generation||1);generation++){
      rows.filter(r=>['01','03','04'].includes(r.id)).forEach(r=>expanded.push({...r,id:r.id+'-g'+generation,age:1,sourceGeneration:generation}));
    }
    return expanded.map((r,i)=>{
      const generation=r.sourceGeneration;
      const suffix=generation>1?' · g'+generation:'';
      const operation=r.service==='gateway'?'GET /v1/health':r.service==='order-api'?'POST /v1/orders':'POST /v1/payments';
      return {...r,id:p.sourceBinding+'-'+r.id,originId:r.id,provider:p.sourceBinding,
        duration:r.duration+(p.sourceBinding==='jaeger'?35:0),
        sourceId:p.sourceBinding+':'+r.resource+':g'+generation,generation,instance:r.service+'@'+r.resource+suffix,endpoint:operation,operation,
        tag:'http.method='+operation.split(' ')[0],kind:'SERVER',scope:'opentelemetry.instrumentation.http',
        file:'/var/log/'+r.service+'/application.log',segmentId:'segment-'+r.id,
        traceProvider:'otel',traceId:'trace-'+r.id,
        resourceName:M.resources.find(x=>x.id===r.resource)?.name||r.resource};
    });
  }
  function apply(p,ctx,rows){
    if(!enabled(p))return rows;
    const state=filterState(p,ctx);
    return rows.filter(r=>definitions(p).every(key=>state[key]==='All'||String(r[key])===state[key])&&
      (!state.keyword||[r.id,r.service,r.operation,r.body].join(' ').toLowerCase().includes(state.keyword.toLowerCase()))&&
      (p.signal!=='traces'||r.duration>=Math.max(0,Number(state.min)||0)));
  }
  function choices(p,key,ctx,vars,scene='normal'){
    let rows=M.panelBaseData(p,ctx,vars,scene).rows;
    const selected=filterState(p,ctx);
    if(key!=='service'&&selected.service!=='All')rows=rows.filter(r=>r.service===selected.service);
    return [...new Set(rows.map(r=>String(r[key]??'')).filter(Boolean))].sort();
  }
  function normalizePanel(p,ctx,vars,scene='normal'){
    if(!enabled(p)||scene==='unavailable')return [];
    ctx.localFilters??={};
    const state=filterState(p,ctx),changes=[];
    ctx.localFilters[p.id]=state;
    definitions(p).forEach(key=>{
      if(state[key]!=='All'&&!choices(p,key,ctx,vars,scene).includes(state[key])){
        changes.push({panel:p.id,key,old:state[key]});state[key]='All';
      }
    });
    return changes;
  }
  function normalize(data,ctx,scene){return data.panels.flatMap(p=>normalizePanel(p,ctx,data.variables,scene))}
  function selectProvider(p,id){
    if(!providers[id]||providers[id].signal!==p.signal)return;
    if(p.sourceBinding!==id)p.roundTrip=null;
    p.sourceBinding=id;p.localDefaults={};p.localDefaults=filterState(p,{});
    if(p.signal==='metrics'){
      p.builder.metric=id==='hostmetrics'?'system_cpu_utilization':'http_requests_total';
      p.builder.op=id==='hostmetrics'?'avg':'rate';p.unit=id==='hostmetrics'?'%':'req/s';
    }
    if(p.mode==='builder')p.expression=M.expression(p);
  }
  function upgrade(state){
    if(!M.actors.viewer.dashboards.includes('source-workbench'))M.actors.viewer.dashboards.push('source-workbench');
    if(state.dashboards.some(b=>b.id==='source-workbench'))return;
    const base=M.copy(state.dashboards.find(b=>b.id==='payment')||M.seed().dashboards[0]);
    const defs=[
      ['source-sw','SkyWalking · 服务与链路','SkyWalking · services & traces','skywalking',{x:0,y:0,w:6,h:15}],
      ['source-jaeger','Jaeger · 请求与操作','Jaeger · requests & operations','jaeger',{x:6,y:0,w:6,h:15}],
      ['source-metrics','请求速率 · Prometheus','Request rate · Prometheus','prometheus',{x:0,y:15,w:6,h:13}],
      ['source-logs','应用日志 · Filelog','Application logs · Filelog','filelog',{x:6,y:15,w:6,h:13}]
    ];
    base.id='source-workbench';base.name='多来源与两层过滤';base.nameEn='Sources & layered filters';
    base.description='顶部统一时间和资源；各统计图按来源独立筛选。示例数据，不连接厂商后端。';
    base.descriptionEn='Shared time and resources, with independent source-specific panel filters. Sample data only.';
    base.revision=1;base.defaultRange='1h';
    base.variables=base.variables.filter(v=>v.key==='environment');
    base.variables.forEach(v=>{v.defaultValues=['All'];v.dependency=''});
    base.panels=defs.map(([id,zh,en,source,layout])=>{
      const signal=providers[source].signal,p=M.panel(id,signal,zh,en,signal==='traces'?'traces':signal==='logs'?'records':'rate');
      p.sourceBinding=source;p.localDefaults={...defaults};p.layout=layout;
      p.builder.variable='';p.builder.environmentVariable='environment';
      if(signal==='logs')p.builder.severity='ANY';
      if(D.apm)p.drilldowns=D.apm.standardDrilldowns(p);
      p.expression=M.expression(p);p.unit=signal==='traces'?'ms':signal==='logs'?'':'req/s';
      return p;
    });
    state.dashboards.unshift(base);M.persist();
  }
  D.sources={providers,defaults,provider,enabled,definitions,filterState,materialize,apply,choices,normalizePanel,normalize,selectProvider,upgrade};
})();
