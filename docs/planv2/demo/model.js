(() => {
  'use strict';
  const D = window.ArgusDemo;
  const KEY = 'argus.planv2.review.v4';
  const copy = value => JSON.parse(JSON.stringify(value));
  const actors = {
    editor: {id:'editor', manage:true, resources:['host-a','host-b','cluster-a'], dashboards:['payment','infra','orders']},
    viewer: {id:'viewer', manage:false, resources:['host-a','cluster-a'], dashboards:['payment','infra']}
  };
  const resources = [
    {id:'host-a', type:'host', name:'host-pay-01', address:'10.20.1.14'},
    {id:'host-b', type:'host', name:'host-pay-02', address:'10.20.1.15'},
    {id:'cluster-a', type:'cluster', name:'prod-cn-shanghai', address:'Kubernetes · 3 nodes'}
  ];
  const catalog = {
    metrics:[
      {name:'http_requests_total',type:'Counter',source:'OTLP',fields:['environment','service','status']},
      {name:'http_request_duration_seconds_bucket',type:'Histogram',source:'OTLP',fields:['environment','service','le']},
      {name:'system_cpu_utilization',type:'Gauge',source:'hostmetrics',fields:['environment','service','host']},
      {name:'system_memory_usage',type:'Gauge',source:'hostmetrics',fields:['environment','host']}
    ],
    logs:[
      {name:'service_name',type:'String',source:'OTLP',fields:['service']},
      {name:'severity_text',type:'String',source:'filelog',fields:['severity']},
      {name:'environment',type:'String',source:'OTLP',fields:['environment']},
      {name:'body',type:'String',source:'filelog',fields:['message']}
    ],
    traces:[
      {name:'serviceName',type:'String',source:'OTLP',fields:['service']},
      {name:'status',type:'Enum',source:'OTLP',fields:['status']},
      {name:'durationMin',type:'Milliseconds',source:'OTLP',fields:['duration']},
      {name:'environment',type:'String',source:'OTLP',fields:['environment']}
    ]
  };
  const observations = [
    {id:'01',resource:'host-a',environment:'production',service:'payment-api',severity:'ERROR',age:3,body:'risk provider timeout',duration:840,status:'ERROR'},
    {id:'02',resource:'host-a',environment:'production',service:'payment-api',severity:'WARN',age:7,body:'retry budget near limit',duration:420,status:'OK'},
    {id:'03',resource:'host-b',environment:'production',service:'payment-api',severity:'ERROR',age:5,body:'ledger deadline exceeded',duration:1260,status:'ERROR'},
    {id:'04',resource:'cluster-a',environment:'production',service:'gateway',severity:'INFO',age:8,body:'request completed',duration:180,status:'OK'},
    {id:'05',resource:'host-a',environment:'staging',service:'payment-canary',severity:'INFO',age:38,body:'deployment health check passed',duration:210,status:'OK'},
    {id:'06',resource:'host-b',environment:'staging',service:'order-api',severity:'ERROR',age:43,body:'upstream connection timeout',duration:920,status:'ERROR'},
    {id:'07',resource:'cluster-a',environment:'staging',service:'order-api',severity:'WARN',age:51,body:'worker queue retry',duration:510,status:'OK'},
    {id:'08',resource:'host-a',environment:'production',service:'payment-api',severity:'ERROR',age:22,body:'database connection timeout',duration:690,status:'ERROR'}
  ];
  const ranges = {'15m':15,'1h':60,'6h':360,'24h':1440,yesterday:1440,custom:60};
  for(let i=0;i<36;i++){
    const staging=i%7===0,severity=i%5===0?'ERROR':i%3===0?'WARN':'INFO';
    observations.push({id:'sample-'+i,resource:['host-a','host-b','cluster-a'][i%3],environment:staging?'staging':'production',service:staging?'payment-canary':['payment-api','order-api','gateway'][i%3],severity,age:staging?35+i%20:1+(i*7)%55,body:severity==='ERROR'?'upstream timeout while processing /v1/payments':severity==='WARN'?'retry request after connection reset':'request completed successfully',duration:severity==='ERROR'?700+i*17:80+i*9,status:severity==='ERROR'?'ERROR':'OK'});
  }
  const operations = {metrics:['rate','sum','avg','max','topk','error_rate','p95'],logs:['records','count','trend','group_count'],traces:['traces','slow','errors','detail']};
  const types = {metrics:['time_series','stat','gauge','bar_gauge','bar_chart','pie','histogram','heatmap','state_timeline','scatter','table'],logs:['logs','table','time_series','stat','bar_chart','pie'],traces:['trace_list','trace_detail','apm_overview','apm_topology','apm_endpoints']};
  function allowedTypes(p){
    if(p.signal!=='logs')return types[p.signal];
    const op=effectiveBuilder(p)?.op;
    return !op?types.logs:op==='records'?['logs','table']:op==='trend'?['time_series','table']:['stat','table','bar_chart','pie'];
  }
  function builder(signal, op) {
    return {metric:signal==='metrics'?'http_requests_total':'',op:op || operations[signal][0],group:'service',window:'5m',variable:'service',environmentVariable:'',severity:'ERROR',duration:400,limit:100};
  }
  function compile(signal,b) {
    const ref = b.variable ? '$'+b.variable : '';
    if(signal==='metrics'){
      const filters=[]; if(ref)filters.push('service=~"'+ref+'"'); if(b.environmentVariable)filters.push('environment=~"$'+b.environmentVariable+'"');
      const match = filters.length ? '{'+filters.join(',')+'}' : '';
      const m=b.metric+match, r='rate('+m+'['+b.window+'])';
      if(b.op==='p95')return 'histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket'+match+'['+b.window+'])) by (le, service))';
      if(b.op==='error_rate')return 'sum(rate(http_requests_total{'+filters.concat('status=~"5.."').join(',')+'}['+b.window+'])) / sum(rate(http_requests_total'+match+'['+b.window+']))';
      if(b.op==='topk')return 'topk(5, sum('+r+') by ('+b.group+'))';
      if(b.op==='rate')return 'sum('+r+') by ('+b.group+')';
      return b.op+'('+m+') by ('+b.group+')';
    }
    if(signal==='logs'){
      const parts=[];if(ref)parts.push('service_name = "'+ref+'"');if(b.environmentVariable)parts.push('stream_labels.environment = "$'+b.environmentVariable+'"');if(b.severity!=='ANY')parts.push('severity_text = "'+b.severity+'"');const filter=parts.join(' AND ')||'body: *';
      if(b.op==='group_count')return filter+' | stats count() by service_name';
      if(b.op==='count')return filter+' | stats count() by severity_text';
      if(b.op==='trend')return filter+' | stats count() by timestamp'; // Illustrative builder output, never executed.
      return filter+' | limit '+b.limit;
    }
    const args = [];
    if(ref) args.push('serviceName: "'+ref+'"');
    if(b.environmentVariable)args.push('tags: [{key: "environment", value: "$'+b.environmentVariable+'"}]');
    if(b.op==='slow')args.push('durationMin: '+b.duration);
    if(b.op==='errors')args.push('status: "ERROR"');
    args.push('pageSize: '+Math.min(b.limit,100));
    const fields=b.op==='detail'?'traceId rootService duration status spans { spanId parentSpanId operationName duration status }':'traceId rootService rootOperation duration status';
    return 'query { queryTraces('+args.join(', ')+') { total traces { '+fields+' } } }';
  }
  function panel(id,signal,title,titleEn,op,type) {
    const b=builder(signal,op);
    const p={id,signal,title,titleEn,type:type||types[signal][0],mode:'builder',builder:b,expression:compile(signal,b),roundTrip:null,width:6,resources:['host','cluster'],unit:signal==='metrics'?'req/s':signal==='traces'?'ms':'',description:'',descriptionEn:''};
    if(D.apm)p.drilldowns=D.apm.standardDrilldowns(p);
    return p;
  }
  function seed(){
    const panels=[
      panel('traffic','metrics','请求速率','Request rate','rate'),
      panel('latency','metrics','P95 延迟','P95 latency','p95'),
      panel('errors','logs','错误日志','Error logs','records','logs'),
      panel('traces','traces','慢 Trace','Slow traces','slow','trace_list')
    ];
    panels[1].unit='ms';
    panels[1].builder.metric='http_request_duration_seconds_bucket';
    panels.forEach(p=>{p.builder.environmentVariable='environment';p.expression=compile(p.signal,p.builder)});
    const variables=[
      {key:'environment',label:'环境',labelEn:'Environment',signal:'metrics',item:'http_requests_total',field:'environment',dependency:'',selection:'single',defaultValues:['All'],refresh:'data'},
      {key:'service',label:'服务',labelEn:'Service',signal:'metrics',item:'http_requests_total',field:'service',dependency:'environment',selection:'multi',defaultValues:['All'],refresh:'data'}
    ];
    const cpu=panel('cpu','metrics','CPU 使用率','CPU utilization','avg');cpu.builder.metric='system_cpu_utilization';cpu.builder.variable='';cpu.expression=compile('metrics',cpu.builder);cpu.resources=['host'];cpu.unit='%';
    const hostlogs=panel('hostlogs','logs','主机日志','Host logs','records','logs');hostlogs.builder.variable='';hostlogs.expression=compile('logs',hostlogs.builder);
    return {
      version:4,locale:'zh-CN',theme:'dark',actor:'editor',review:{},notes:{},
      folders:[{id:'prod',name:'生产服务',nameEn:'Production services'},{id:'infra',name:'基础设施',nameEn:'Infrastructure'}],
      dashboards:[
        {id:'payment',name:'支付服务健康度',nameEn:'Payment service health',description:'统一观察请求、延迟、日志和 Trace。',descriptionEn:'Requests, latency, logs and traces in one place.',folder:'prod',revision:7,panels,variables,defaultRange:'1h',defaultRefresh:0,updated:Date.now()},
        {id:'infra',name:'主机运行概览',nameEn:'Host operations',description:'主机资源与运行日志。',descriptionEn:'Host resources and operational logs.',folder:'infra',revision:3,panels:[cpu,hostlogs],variables:[],defaultRange:'1h',defaultRefresh:0,updated:Date.now()},
        {id:'orders',name:'订单链路概览',nameEn:'Order service overview',description:'订单服务的观察模板。',descriptionEn:'An observability template for order services.',folder:null,revision:2,panels:[],variables:[],defaultRange:'1h',defaultRefresh:0,updated:Date.now()}
      ],
      drafts:{},bindings:{'host-a':['payment','infra'],'host-b':['payment'],'cluster-a':['payment']},
      chat:{editor:{messages:[],context:null,selected:[]},viewer:{messages:[],context:null,selected:[]}}
    };
  }
  function load(){try{const v=JSON.parse(localStorage.getItem(KEY));return v?.version===4?v:seed()}catch{return seed()}}
  D.state=load();
  function persist(){try{localStorage.setItem(KEY,JSON.stringify(D.state));return true}catch{D.toast?.(D.t('storageError'));return false}}
  function actor(){return actors[D.state.actor]}
  function allowedResources(){return resources.filter(r=>actor().resources.includes(r.id))}
  function canRead(id){return actor().manage || actor().dashboards.includes(id)}
  function boards(){return D.state.dashboards.filter(x=>canRead(x.id)&&x.lifecycle!=='archived')}
  function board(id){return D.state.dashboards.find(x=>x.id===id && canRead(id))}
  function label(object, field='name'){return D.locale==='en-US' ? object[field+'En']||object[field] : object[field]}
  function draftKey(id){return actor().id+':'+id}
  function getDraft(id){return D.state.drafts[draftKey(id)]}
  function beginDraft(id){
    if(!actor().manage)return null;
    if(getDraft(id))return getDraft(id);
    const source=board(id);if(!source||source.lifecycle==='archived')return null;
    const draft={id,owner:actor().id,base:source.revision,baseObjectVersion:source.objectVersion||0,version:1,saved:Date.now(),data:copy(source),origin:'ui'};
    D.state.drafts[draftKey(id)]=draft;persist();return draft;
  }
  function createDraft(name,desc,folder,origin='ui',mode='builder'){
    if(!actor().manage)return null;
    const id='draft-'+Date.now().toString(36);
    const base=origin==='ai'?copy(D.state.dashboards[0]):{panels:[],variables:[],defaultRange:'1h',defaultRefresh:0};
    const data={...base,id,name,nameEn:name,description:desc,descriptionEn:desc,folder,revision:0};
    if(mode==='dsl')data.panels.forEach(p=>setMode(p,'dsl'));
    const draft={id,owner:actor().id,base:null,version:1,saved:Date.now(),data,origin};
    D.state.drafts[draftKey(id)]=draft;persist();return draft;
  }
  function save(draft){draft.version++;draft.saved=Date.now();persist()}
  function expression(p){return p.mode==='builder'?compile(p.signal,p.builder):p.expression}
  function queryContext(p){return JSON.stringify([p.signal,p.sourceBinding||'generic',p.capabilityVersion||'demo/v1'])}
  function effectiveBuilder(p){return p.mode==='builder'?p.builder:canBuild(p)?p.roundTrip.builder:null}
  function canBuild(p){return p.mode==='builder' || !!(p.roundTrip && p.expression===p.roundTrip.expression && p.roundTrip.context===queryContext(p))}
  function setMode(p,mode){
    if(mode===p.mode)return true;
    if(mode==='dsl'){const text=compile(p.signal,p.builder);p.roundTrip={expression:text,builder:copy(p.builder),context:queryContext(p)};p.expression=text;p.mode='dsl';return true}
    if(!canBuild(p))return false;
    p.builder=copy(p.roundTrip.builder);p.mode='builder';return true;
  }
  function variableErrors(vars){
    const keys=vars.map(v=>v.key), errors=[];
    if(keys.some((x,i)=>!/^[a-zA-Z_]\w*$/.test(x)||keys.indexOf(x)!==i))errors.push('variableInvalid');
    const visit=(key,path=[])=>{
      if(path.includes(key)){errors.push('variableInvalid');return}
      const v=vars.find(x=>x.key===key);if(v?.dependency){if(!keys.includes(v.dependency))errors.push('variableInvalid');else visit(v.dependency,path.concat(key))}
    };keys.forEach(key=>visit(key));
    return [...new Set(errors)];
  }
  // Full placeholder tokens. GraphQL native variables outside quoted templates stay separate.
  function refs(p){
    if(p.mode==='builder')return [...new Set([p.builder.variable,p.builder.environmentVariable].filter(Boolean))];
    const q=expression(p),found=new Set();let quote='',escaped=false,comment=false;
    for(let i=0;i<q.length;i++){
      const c=q[i];
      if(comment){if(c==='\n')comment=false;continue}
      if(escaped){escaped=false;continue}
      if(c==='\\'&&quote){escaped=true;continue}
      if(!quote&&c==='#'){comment=true;continue}
      if(c==='"'||c==="'"){if(quote===c)quote='';else if(!quote)quote=c;continue}
      if(c==='$'&&(p.signal!=='traces'||quote)){
        const m=q.slice(i).match(/^\$(?:\{([A-Za-z_]\w*)\}|([A-Za-z_]\w*))/);
        if(m){found.add(m[1]||m[2]);i+=m[0].length-1}
      }
    }
    return [...found];
  }
  function uses(p,key){return refs(p).includes(key)}
  function validExpression(p){
    const q=expression(p);if(q.length<3)return false;
    const stack=[];let quote='',escape=false;
    for(const c of q){
      if(escape){escape=false;continue}
      if(c==='\\' && quote){escape=true;continue}
      if(quote){if(c===quote)quote='';continue}
      if(c==='"'||c==="'"){quote=c;continue}
      if('([{'.includes(c))stack.push(c);
      if(')]}'.includes(c) && stack.pop()!==({')':'(',']':'[','}':'{'})[c])return false;
    }
    if(stack.length||quote)return false;
    if(p.signal==='traces')return /^\s*(query\b|\{)/.test(q);
    if(p.signal==='logs')return /service_name|severity_text|body|environment/.test(q);
    return !/^\s*(query\b|\{)/.test(q);
  }
  function validate(data){
    const errors=variableErrors(data.variables);
    if(!data.name.trim())errors.push('name');
    if(D.state.folders.find(f=>f.id===data.folder)?.status==='archived')errors.push(D.local('目标分组已归档','The destination folder is archived'));
    for(const p of data.panels){
      if(!p.title.trim() || !validExpression(p) || !allowedTypes(p)?.includes(p.type) || !p.resources?.length) errors.push(p.title+': '+D.t('invalidQuery'));
      for(const key of refs(p))if(!data.variables.some(v=>v.key===key))errors.push(p.title+': '+D.local('未定义变量 $','Undefined variable $')+key);
      if(p.sourceBinding==='hostmetrics' && /http_requests_total|http_request_duration_seconds_bucket/.test(expression(p)))errors.push(p.title+': '+D.local('Host metrics 来源不支持此请求指标，请修改查询。','Host metrics does not provide this request metric. Update the query.'));
      if(D.apm)errors.push(...D.apm.validate(p));
      if(p.mode==='builder' && p.builder.op==='p95' && !p.builder.metric.includes('bucket'))errors.push(p.title+': Histogram');
    }
    return [...new Set(errors)];
  }
  function preview(draft,scene){return {id:draft.id,version:draft.version,base:draft.base,baseObjectVersion:draft.baseObjectVersion||0,data:copy(draft.data),scene,errors:validate(draft.data)}}
  function publish(preview){
    if(!actor().manage)return {error:'unauthorized'};
    const draft=getDraft(preview.id);if(!draft || draft.version!==preview.version)return {error:'invalidated'};
    const current=board(preview.id);
    if(current?.lifecycle==='archived'||D.state.folders.find(f=>f.id===preview.data.folder)?.status==='archived')return {error:'unavailable'};
    if((current?.objectVersion||0)!==(preview.baseObjectVersion||0))return {error:'conflict'};
    if((current?.revision??null)!==preview.base)return {error:'conflict'};
    if(validate(preview.data).length)return {error:'blocked'};
    const data=copy(preview.data);data.revision=(current?.revision||0)+1;data.objectVersion=(current?.objectVersion||0)+1;data.lifecycle='active';data.updated=Date.now();
    if(current)D.state.dashboards[D.state.dashboards.indexOf(current)]=data;else D.state.dashboards.push(data);
    delete D.state.drafts[draftKey(draft.id)];persist();return {data,origin:draft.origin};
  }
  function conflict(id){const b=board(id);if(!b)return;b.revision++;b.description=D.t('normalDescription');b.descriptionEn=b.description;persist()}
  function context(data){const vars={};data.variables.forEach(v=>vars[v.key]=copy(v.defaultValues?.length?v.defaultValues:['All']));return {range:data.defaultRange||'1h',resource:'all',vars,localFilters:{},refresh:data.defaultRefresh||0,from:'',to:''}}
  function duration(ctx){if(ctx.range!=='custom')return ranges[ctx.range]||60;const delta=(Date.parse(ctx.to)-Date.parse(ctx.from))/60000;return Number.isFinite(delta)&&delta>0?delta:0}
  function timeWindow(ctx){
    let end=ctx.snapshotEnd||Date.now(),start=end-duration(ctx)*60000;
    if(ctx.range==='custom'){start=Date.parse(ctx.from);end=Date.parse(ctx.to)}
    if(ctx.range==='yesterday'){const day=new Date(end);day.setHours(0,0,0,0);end=day.getTime();day.setDate(day.getDate()-1);start=day.getTime()}
    return {from:new Date(start).toISOString(),to:new Date(end).toISOString(),end};
  }
  function scopedRows(ctx,scene='normal',withTime=true){
    if(scene==='no_data')return [];
    const ids=allowedResources().filter(r=>ctx.resource==='all'||r.id===ctx.resource).map(r=>r.id);
    return observations.filter(row=>ids.includes(row.resource) && (!withTime||row.age<=duration(ctx))).sort((a,b)=>a.age-b.age);
  }
  function options(v,vars,ctx,scene='normal',visited=[]){
    if(visited.includes(v.key))return [];
    let rows=scopedRows(ctx,scene);
    if(v.dependency){
      const parent=vars.find(x=>x.key===v.dependency),selected=ctx.vars[v.dependency]||['All'];
      if(parent && !selected.includes('All'))rows=rows.filter(row=>selected.includes(String(row[parent.field])));
    }
    return [...new Set(rows.map(row=>String(row[v.field]??'')).filter(Boolean))];
  }
  function normalizeContext(ctx,vars,scene='normal'){
    const changes=[],seen=new Set();
    const apply=v=>{
      if(seen.has(v.key))return;seen.add(v.key);
      if(v.dependency){const p=vars.find(x=>x.key===v.dependency);if(p)apply(p)}
      const selected=ctx.vars[v.key]||copy(v.defaultValues?.length?v.defaultValues:['All']);
      const available=options(v,vars,ctx,scene);
      if(!selected.includes('All') && selected.some(x=>!available.includes(x))){changes.push({name:label(v,'label'),old:selected.join(', ')});ctx.vars[v.key]=['All']}
      else ctx.vars[v.key]=selected.length?selected:['All'];
    };vars.forEach(apply);return changes;
  }
  function panelBaseData(p,ctx,vars,scene='normal'){
    const b=effectiveBuilder(p);
    const scope=allowedResources().filter(r=>ctx.resource==='all'||ctx.resource===r.id);
    if(!scope.some(r=>p.resources.includes(r.type)))return {status:'notApplicable',rows:[]};
    if(scene==='unavailable')return {status:'unavailable',rows:[]};
    let rows=scopedRows(ctx,scene,false).filter(row=>p.resources.includes(resources.find(r=>r.id===row.resource)?.type));
    if(D.sources)rows=D.sources.materialize(p,rows,ctx);
    rows=rows.filter(row=>row.age<=duration(ctx)).sort((a,b)=>a.age-b.age);
    // Custom filters apply only where a panel explicitly uses a variable; system scope always applies.
    vars.forEach(v=>{
      const used=uses(p,v.key);
      const selected=ctx.vars[v.key]||['All'];
      if(used && !selected.includes('All')){
        const fields=b?[...(b.variable===v.key?['service']:[]),...(b.environmentVariable===v.key?['environment']:[])]:[];
        fields.forEach(field=>{rows=rows.filter(row=>selected.includes(String(row[field])))});
      }
    });
    if(p.signal==='logs' && b && b.severity!=='ANY')rows=rows.filter(r=>r.severity===b.severity);
    if(p.signal==='traces' && b){
      if(b.op==='slow')rows=rows.filter(r=>r.duration>=b.duration);
      if(b.op==='errors')rows=rows.filter(r=>r.status==='ERROR');
    }
    return {status:rows.length?'complete':'noData',rows};
  }
  function panelData(p,ctx,vars,scene='normal'){
    const base=panelBaseData(p,ctx,vars,scene),b=effectiveBuilder(p);
    if(['notApplicable','unavailable'].includes(base.status))return base;
    let rows=D.sources?D.sources.apply(p,ctx,base.rows):base.rows;
    if(b && (p.signal==='traces'||p.signal==='logs'&&b.op==='records'))rows=rows.slice(0,Math.max(1,Number(b.limit)||100));
    if(!rows.length)return {status:'noData',rows:[]};
    const total=rows.length;if(scene==='partial')rows=rows.slice(0,Math.max(1,Math.floor(total/2)));
    return {status:scene==='partial'?'partial':'complete',rows:copy(rows),total};
  }
  function files(data,ctx,question,scene){
    const time=timeWindow(ctx);
    const signal=ctx.panelSignal||(/只看日志|logs only/i.test(question)?'logs':/只看.*trace|traces only/i.test(question)?'traces':/只看.*(指标|cpu)|metrics only/i.test(question)?'metrics':'');
    return data.panels.filter(p=>!signal||p.signal===signal).map(p=>{
      const result=panelData(p,ctx,data.variables,scene);
      let contentRows=result.rows.map(r=>({...r,timestamp:new Date(time.end-r.age*60000).toISOString()}));
      if(p.signal==='metrics')contentRows=result.rows.map((r,i)=>({series:{service:r.service,resource:r.resource},timestamp:new Date(time.end-r.age*60000).toISOString(),value:Number((p.builder.op==='p95'?r.duration:10+i*7.5).toFixed(2)),unit:p.unit}));
      if(p.signal==='traces')contentRows=result.rows.map(r=>({traceId:r.traceId||r.id,rootService:r.service,startTime:new Date(time.end-r.age*60000).toISOString(),duration:r.duration,status:r.status,...(/\bspans\s*\{/.test(expression(p))?{spans:[{spanId:r.id+'a',duration:r.duration}]}:{})}));
      if(p.signal==='logs' && ['count','group_count','trend'].includes(effectiveBuilder(p)?.op))contentRows=logAggregate(p,result.rows,ctx);
      return {dashboard:data.id,panel:p.id,title:label(p,'title'),signal:p.signal,source:p.sourceBinding||'legacy',localFilters:D.sources?.enabled(p)?D.sources.filterState(p,ctx):{},status:result.status,total:result.total||0,count:contentRows.length,revision:data.revision,time:{from:time.from,to:time.to},query:expression(p),path:'/workspace/telemetry/'+(ctx.executionId||'demo')+'/'+data.id+'/'+p.id+'.jsonl',content:contentRows.map(r=>JSON.stringify(r)).join('\n')};
    });
  }
  function logAggregate(p,rows,ctx){
    const op=effectiveBuilder(p)?.op;
    if(op==='trend'){
      const step=duration(ctx)/12;
      return Array.from({length:12},(_,i)=>{const hi=(12-i)*step,lo=hi-step;return {bucketMinutes:Number(hi.toFixed(3)),count:rows.filter(r=>r.age>lo&&r.age<=hi).length}});
    }
    const key=op==='group_count'?'service':'severity',values=[...new Set(rows.map(r=>r[key]))];
    return values.map(v=>({group:v,count:rows.filter(r=>r[key]===v).length}));
  }
  D.model={KEY,copy,actors,resources,catalog,operations,types,allowedTypes,seed,persist,actor,allowedResources,canRead,boards,board,label,draftKey,getDraft,beginDraft,createDraft,save,expression,effectiveBuilder,canBuild,setMode,queryContext,refs,uses,variableErrors,validExpression,validate,preview,publish,conflict,context,options,normalizeContext,panelBaseData,panelData,files,builder,panel,duration,logAggregate};
})();
