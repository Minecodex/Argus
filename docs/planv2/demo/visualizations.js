(() => {
  'use strict';
  const D=window.ArgusDemo,M=D.model;
  const tr=(zh,en)=>D.local(zh,en),esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  D.addStrings({
    reviewed:['已核对 {n} / {total}','Reviewed {n} / {total}'],reviewIntro:['{n} 个可体验变化点；本轮新增丰富图型、日志探索、Trace 瀑布与自由网格。','{n} interactive changes, now including richer charts, log exploration, trace waterfalls and a free grid.'],
    layoutHelp:['拖动标题栏左侧把手移动，拖右下角调整宽高；12 列网格吸附，碰撞面板向下让位。变更只进入草稿。','Drag the title handle to move, or the bottom-right corner to resize. A 12-column grid snaps panels and pushes collisions down. Changes remain in your draft.'],
    layoutKeyboard:['方向键移动；缩放把手方向键改尺寸；Shift 加速；Esc 取消拖动。','Arrow keys move or resize; Shift accelerates; Esc cancels a drag.'],
    movePanel:['拖动统计图：{name}','Move panel: {name}'],resizePanel:['调整统计图尺寸：{name}','Resize panel: {name}'],
    inspectData:['查看数据','Inspect data'],explore:['展开探索','Explore'],
    'type.gauge':['仪表盘 Gauge','Gauge'],'type.bar_gauge':['条形仪表','Bar gauge'],'type.bar_chart':['柱状 / 条形图','Bar chart'],'type.pie':['饼图 / 环图','Pie / donut'],'type.histogram':['直方图','Histogram'],'type.heatmap':['热力图','Heatmap'],'type.state_timeline':['状态时间线','State timeline'],'type.scatter':['散点图','Scatter'],
    visualOptions:['可视化与布局','Visualization & layout'],widthCols:['宽度（列）','Width (columns)'],heightRows:['高度（行）','Height (rows)'],positionX:['横向位置（列）','X (column)'],positionY:['纵向位置（行）','Y (row)'],decimals:['小数位','Decimals'],reducer:['显示计算','Value calculation'],minValue:['最小值','Minimum'],maxValue:['最大值','Maximum'],showLegend:['显示图例','Show legend'],stacking:['堆叠序列','Stack series'],smooth:['平滑折线','Smooth line'],lineStyle:['图形样式','Draw style'],viewData:['结果数据（样例）','Result data (sample)'],
    logExplore:['日志探索','Log exploration'],traceExplore:['Trace 探索','Trace exploration'],keyword:['在当前结果中搜索','Search current results'],level:['级别','Level'],sort:['排序','Order'],newest:['最新优先','Newest first'],oldest:['最早优先','Oldest first'],wrap:['自动换行','Wrap lines'],logVolume:['日志量（当前结果）','Log volume (current results)'],fields:['字段 / 标签','Fields / labels'],contextLines:['查看上下文','Show context'],viewTrace:['查看关联 Trace','Open linked trace'],localFilters:['临时筛选只影响当前结果，不改发布查询或 Chat 条件。','Temporary filters affect these results only, not the published query or Chat context.'],clearFilters:['清除临时筛选','Clear temporary filters'],spanDetails:['Span 详情','Span details'],waterfall:['Span 瀑布','Span waterfall'],attributes:['属性','Attributes'],events:['事件','Events'],traceSearch:['服务、端点或 Trace ID','Service, endpoint or Trace ID'],minDuration:['最小时延 ms','Minimum duration ms'],traceDistribution:['时延分布（当前结果）','Duration distribution (current results)'],errorOnly:['只看错误','Errors only'],spanSearch:['搜索 Span / 服务','Search span or service'],showcase:['丰富展示与自由布局','Rich visualization & free layout'],backendPending:['示例交互；完整 LogQL / 链路后端能力正在重新讨论。','Example interactions. Full LogQL and tracing backend capabilities are under discussion.']
  });
  D.changes.push(
    {id:'rich-metrics',refs:'UI / Metrics',zh:'11 种 Metrics 展示',en:'11 metric visualizations',before:['趋势、统计值和基础表格。','A basic line, stat and table.'],after:['新增 Gauge、条形仪表、柱图、环图、直方图、热力图、状态时间线与散点图，支持图例和缩放。','Add gauges, bars, donut, histogram, heatmap, state timeline and scatter with legends and zoom.']},
    {id:'rich-logs',refs:'UI / Logs',zh:'日志量、检索、字段与上下文',en:'Log volume, search, fields and context',before:['只有几行日志表格。','Only a small log table.'],after:['按关键字、级别与标签筛选，展开结构化字段，查看上下文及关联 Trace。','Filter by text, level and labels; inspect fields, surrounding context and linked traces.']},
    {id:'rich-traces',refs:'UI / Traces',zh:'链路列表与 Span 瀑布',en:'Trace search and span waterfall',before:['时延条和 JSON 详情。','Duration bars and JSON details.'],after:['服务 / ID / 时延筛选，打开层级 Span 瀑布，查看属性与事件；真实完整链路需补数据链路。','Search traces, inspect a hierarchical waterfall, attributes and events. Full production tracing needs backend work.']},
    {id:'free-layout',refs:'UI / Grid',zh:'真实拖动和任意网格尺寸',en:'Free movement and grid resizing',before:['只能切半宽 / 全宽或调整顺序。','Half/full width toggles and ordering only.'],after:['拖标题移动、拖右下角缩放，按 x/y/w/h 保存，占用 3～12 列及独立行高。','Drag to move and resize. Persist x/y/w/h, 3–12 columns and independent heights.']}
  );
  const charts=new Map(),observers=new Map();
  let modalCharts=[];
  function colors(){const s=getComputedStyle(document.documentElement),read=x=>s.getPropertyValue(x).trim();return {text:read('--text-secondary'),muted:read('--text-tertiary'),line:read('--border-default'),bg:read('--bg-surface'),font10:parseFloat(read('--font-size-10')),font24:parseFloat(read('--font-size-24')),palette:['--info','--brand-highlight','--success','--warning','--danger'].map(read)}}
  function display(p){return {decimals:1,reducer:'last',min:0,max:100,legend:true,stack:false,smooth:true,style:'line',...(p.display||{})}}
  function upgrade(state){
    if(!M.actors.viewer.dashboards.includes('showcase'))M.actors.viewer.dashboards.push('showcase');
    if(!state.dashboards.some(b=>b.id==='showcase')){
      const base=M.copy(state.dashboards.find(b=>b.id==='payment'));
      const defs=[
        ['rich-rate','请求与错误趋势','Request and error trends','time_series',{x:0,y:0,w:8,h:9}],
        ['rich-gauge','主机负载','Host load','gauge',{x:8,y:0,w:4,h:9}],
        ['rich-bar','服务请求分布','Requests by service','bar_chart',{x:0,y:9,w:6,h:9}],
        ['rich-heat','请求延迟热力图','Latency heatmap','heatmap',{x:6,y:9,w:6,h:9}],
        ['rich-logs','日志检索与上下文','Log search and context','logs',{x:0,y:18,w:12,h:12}],
        ['rich-traces','慢请求与分布式链路','Slow requests and traces','trace_list',{x:0,y:30,w:12,h:12}]
      ];
      base.id='showcase';base.name='三信号观测工作台';base.nameEn='Three-signal observability';base.description='丰富图型、日志探索、Span 瀑布与自由布局的交互样例。';base.descriptionEn='Rich charts, logs, span waterfalls and freely positioned panels.';base.revision=1;
      base.panels=defs.map(([id,zh,en,type,layout])=>{
        const signal=type==='logs'?'logs':type==='trace_list'?'traces':'metrics',p=M.panel(id,signal,zh,en,signal==='logs'?'records':signal==='traces'?'traces':'rate',type);
        p.builder.variable='service';p.builder.environmentVariable='environment';if(signal==='logs')p.builder.severity='ANY';
        if(type==='gauge'){p.builder.metric='system_cpu_utilization';p.builder.op='avg';p.resources=['host']}
        if(type==='heatmap'){p.builder.metric='http_request_duration_seconds_bucket';p.builder.group='le'}
        p.expression=M.expression(p);p.layout=layout;p.unit=type==='gauge'?'%':type==='heatmap'?'ms':'req/s';p.display=display(p);return p;
      });
      state.dashboards.push(base);
    }
    D.grid.normalizeState(state);
  }
  function series(p,rows){
    const names=[...new Set(rows.map(r=>r.service))].slice(0,4),count=36;
    return names.map((name,k)=>({name,points:Array.from({length:count},(_,i)=>Math.round((25+k*14+i*.55+Math.sin(i*.43+k)*10+rows.length*.8)*100)/100)}));
  }
  function reduce(values,op){return op==='mean'?values.reduce((a,b)=>a+b,0)/values.length:op==='min'?Math.min(...values):op==='max'?Math.max(...values):op==='sum'?values.reduce((a,b)=>a+b,0):values.at(-1)}
  function configuration(p,result){
    if(p.type==='apm_topology')return D.apm.graphOption(p,result);
    if(p.signal==='logs'){
      const c=colors(),data=M.logAggregate(p,result.rows,D.ui.ctx),trend=M.effectiveBuilder(p)?.op==='trend';
      const names=data.map(x=>trend?'−'+x.bucketMinutes+'m':x.group),values=data.map(x=>x.count);
      return {animation:false,color:c.palette,tooltip:{trigger:'item',confine:true},grid:{left:40,right:18,top:20,bottom:40},xAxis:{type:'category',data:names,axisLabel:{color:c.muted}},yAxis:{type:'value',axisLabel:{color:c.muted},splitLine:{lineStyle:{color:c.line}}},series:[p.type==='pie'?{type:'pie',radius:['35%','65%'],label:{color:c.text},data:data.map((x,i)=>({name:names[i],value:x.count}))}:{type:trend?'line':'bar',data:values}]};
    }
    const c=colors(),o=display(p),data=series(p,result.rows),times=Array.from({length:36},(_,i)=>'-'+Math.round((35-i)*M.duration(D.ui.ctx)/35)+'m');
    const base={animation:false,backgroundColor:'transparent',color:c.palette,textStyle:{color:c.text,fontFamily:'Inter, Microsoft YaHei, sans-serif'},tooltip:{trigger:'axis',confine:true,backgroundColor:c.bg,borderColor:c.line,textStyle:{color:c.text}},legend:{show:o.legend,textStyle:{color:c.text},top:0,type:'scroll'},grid:{left:42,right:18,top:o.legend?38:18,bottom:40,containLabel:false},xAxis:{type:'category',data:times,axisLabel:{color:c.muted},axisLine:{lineStyle:{color:c.line}}},yAxis:{type:'value',axisLabel:{color:c.muted},splitLine:{lineStyle:{color:c.line}}},dataZoom:[{type:'inside'},{type:'slider',height:12,bottom:0,borderColor:c.line,textStyle:{color:c.muted}}],series:data.map(x=>({name:x.name,type:o.style==='bar'?'bar':'line',data:x.points,smooth:o.smooth,showSymbol:false,stack:o.stack?'total':undefined,areaStyle:o.style==='area'?{opacity:.22}:undefined}))};
    const values=data.map(x=>({name:x.name,value:Number(reduce(x.points,o.reducer).toFixed(o.decimals))}));
    if(p.type==='gauge')return {animation:false,series:[{type:'gauge',min:o.min,max:o.max,splitNumber:5,startAngle:210,endAngle:-30,itemStyle:{color:c.palette[0]},progress:{show:true,width:16},axisLine:{lineStyle:{width:16,color:[[1,c.line]]}},axisTick:{show:false},splitLine:{length:8,lineStyle:{color:c.muted}},axisLabel:{color:c.muted,distance:14,fontSize:c.font10},pointer:{show:true,itemStyle:{color:c.palette[0]}},anchor:{show:true},title:{show:false},detail:{valueAnimation:false,formatter:'{value} '+p.unit,color:c.text,fontSize:c.font24,offsetCenter:[0,'60%']},data:[{value:values[0]?.value||0}]}]};
    if(p.type==='pie')return {animation:false,color:c.palette,tooltip:{trigger:'item',confine:true},legend:{show:o.legend,bottom:0,textStyle:{color:c.text},type:'scroll'},series:[{type:'pie',radius:['38%','68%'],center:['50%','43%'],avoidLabelOverlap:true,label:{color:c.text,formatter:'{b}\n{d}%'},data:values}]};
    if(p.type==='bar_chart'||p.type==='bar_gauge')return {...base,dataZoom:[],legend:{show:false},grid:{left:115,right:35,top:15,bottom:25},xAxis:{type:'value',max:p.type==='bar_gauge'?o.max:undefined,axisLabel:{color:c.muted},splitLine:{lineStyle:{color:c.line}}},yAxis:{type:'category',data:values.map(x=>x.name),axisLabel:{color:c.text},axisLine:{show:false}},series:[{type:'bar',data:values.map((x,i)=>({value:x.value,itemStyle:{color:c.palette[i%c.palette.length]}})),showBackground:p.type==='bar_gauge',backgroundStyle:{color:c.line},label:{show:true,position:'right',color:c.text},barMaxWidth:24}]};
    if(p.type==='histogram'){
      const bins=Array.from({length:10},(_,i)=>[i*100,result.rows.filter(r=>r.duration>=i*100&&r.duration<(i+1)*100).length+(i<4?5-i:0)]);
      return {...base,legend:{show:false},dataZoom:[],xAxis:{...base.xAxis,data:bins.map(x=>x[0]+'–'+(x[0]+100))},series:[{name:tr('样本数','Samples'),type:'bar',data:bins.map(x=>x[1]),barCategoryGap:1}]};
    }
    if(p.type==='heatmap'){
      const buckets=['0–50','50–100','100–200','200–400','400–800','800+'],cells=[];
      for(let x=0;x<18;x++)for(let y=0;y<6;y++)cells.push([x,y,Math.round(Math.max(0,22-y*3+Math.sin(x*.6+y)*9))]);
      return {...base,legend:{show:false},dataZoom:[],grid:{left:68,right:22,top:12,bottom:56},xAxis:{...base.xAxis,data:times.filter((_,i)=>i%2===0)},yAxis:{type:'category',data:buckets,axisLabel:{color:c.muted},splitArea:{show:false}},visualMap:{min:0,max:32,orient:'horizontal',left:'center',bottom:0,itemHeight:100,itemWidth:8,textStyle:{color:c.muted},inRange:{color:[c.bg,c.palette[0],c.palette[3],c.palette[4]]}},series:[{type:'heatmap',data:cells,emphasis:{itemStyle:{borderColor:c.text,borderWidth:1}}}]};
    }
    if(p.type==='scatter')return {...base,dataZoom:[{type:'inside'}],legend:{show:false},xAxis:{type:'value',name:tr('请求序号','Request index'),axisLabel:{color:c.muted}},series:[{type:'scatter',symbolSize:9,data:result.rows.map((r,i)=>[i,r.duration]),itemStyle:{color:c.palette[0]}}]};
    if(p.type==='state_timeline'){
      return {...base,dataZoom:[],legend:{show:false},grid:{left:105,right:20,top:10,bottom:32},xAxis:{type:'value',min:0,max:60,axisLabel:{color:c.muted,formatter:v=>'-'+(60-v)+'m'},splitLine:{lineStyle:{color:c.line}}},yAxis:{type:'category',data:data.map(x=>x.name),axisLabel:{color:c.text}},series:[{type:'custom',renderItem:(params,api)=>{const start=api.coord([api.value(1),api.value(0)]),end=api.coord([api.value(2),api.value(0)]),h=api.size([0,1])[1]*.62;return {type:'rect',shape:{x:start[0],y:start[1]-h/2,width:end[0]-start[0]-1,height:h},style:{fill:c.palette[api.value(3)]}}},data:data.flatMap((_,i)=>Array.from({length:6},(__,k)=>[i,k*10,(k+1)*10,(i+k)%5===0?4:2]))}]};
    }
    return base;
  }
  function empty(result){return '<div class="argus-empty">'+esc(D.t(result.status))+'</div>'}
  function content(p,result){
    if(!result.rows.length)return empty(result);
    if(D.apm?.supports(p))return D.apm.content(p,result);
    if(p.signal==='traces'&&D.sources?.enabled(p))return D.sourceViews.traceContent(p,result);
    if(p.signal==='metrics'){
      const data=series(p,result.rows),o=display(p),value=reduce(data[0].points,o.reducer);
      if(p.type==='stat')return '<div class="argus-rich-stat"><span class="argus-stat">'+value.toFixed(o.decimals)+' <small>'+esc(p.unit)+'</small></span><span class="argus-muted">'+esc(o.reducer)+' · '+esc(data[0].name)+'</span><div class="argus-viz-host argus-sparkline" data-echart="'+esc(p.id)+'"></div></div>';
      if(p.type==='table')return dataTable(p,result);
      return '<div class="argus-viz-host" data-echart="'+esc(p.id)+'" role="img" aria-label="'+esc(M.label(p,'title'))+'"></div>';
    }
    if(p.signal==='logs'){
      const b=M.effectiveBuilder(p);
      if(p.type==='stat')return '<div class="argus-rich-stat"><span class="argus-stat">'+result.rows.length+'</span><span class="argus-muted">'+esc(tr('匹配日志数（样例）','Matching log count (sample)'))+'</span></div>';
      if(p.type==='time_series'||p.type==='bar_chart'||p.type==='pie')return '<div class="argus-viz-host" data-echart="'+esc(p.id)+'" role="img" aria-label="'+esc(M.label(p,'title'))+'"></div>';
      if(p.type==='table'){
        const agg=b&&b.op!=='records',rows=agg?M.logAggregate(p,result.rows,D.ui.ctx):result.rows;
        return '<div class="argus-table-wrap"><table class="argus-table"><thead><tr><th>'+esc(agg?tr('分组 / 时间桶','Group / bucket'):tr('日志内容','Log message'))+'</th><th>'+esc(agg?tr('数量','Count'):tr('级别','Level'))+'</th></tr></thead><tbody>'+rows.map(r=>'<tr><td>'+esc(agg?r.group??('−'+r.bucketMinutes+'m'):r.body)+'</td><td>'+esc(agg?r.count:r.severity)+'</td></tr>').join('')+'</tbody></table></div>';
      }
      return logView(p,result,false);
    }
    return traceList(p,result,false);
  }
  function dataTable(p,result){
    if(p.signal==='logs')return content({...p,type:'table'},result);
    const o=display(p),data=series(p,result.rows);
    return '<div class="argus-table-wrap"><table class="argus-table"><thead><tr><th>Series</th><th>Last</th><th>Min</th><th>Max</th><th>Mean</th></tr></thead><tbody>'+data.map(x=>'<tr><td>'+esc(x.name)+'</td>'+['last','min','max','mean'].map(op=>'<td class="argus-mono">'+reduce(x.points,op).toFixed(o.decimals)+'</td>').join('')+'</tr>').join('')+'</tbody></table></div>';
  }
  function state(id){D.ui.vizFilters??={};return D.ui.vizFilters[id]??={keyword:'',level:'All',sort:'newest',wrap:true,labels:{},min:0,errors:false}}
  function filterRows(id,rows){
    const s=state(id);
    return rows.filter(r=>(!s.keyword||(r.body+' '+r.service+' '+r.id).toLowerCase().includes(s.keyword.toLowerCase()))&&(s.level==='All'||r.severity===s.level)&&Object.entries(s.labels).every(([key,value])=>String(r[key])===value)&&(!s.errors||r.status==='ERROR')&&r.duration>=Number(s.min||0)).sort((a,b)=>s.sort==='oldest'?b.age-a.age:a.age-b.age);
  }
  function highlight(text,term){if(!term)return esc(text);const i=String(text).toLowerCase().indexOf(term.toLowerCase());return i<0?esc(text):esc(text.slice(0,i))+'<mark>'+esc(text.slice(i,i+term.length))+'</mark>'+esc(text.slice(i+term.length))}
  function tools(p,trace=false){
    if(D.sources?.enabled(p))return '';
    const s=state(p.id);
    return '<div class="argus-explore-tools"><input class="argus-input" data-viz-filter="keyword" data-viz-panel="'+esc(p.id)+'" value="'+esc(s.keyword)+'" placeholder="'+esc(D.t(trace?'traceSearch':'keyword'))+'" aria-label="'+esc(D.t(trace?'traceSearch':'keyword'))+'">'+
      (trace?'<label class="argus-check"><input type="checkbox" data-viz-filter="errors" data-viz-panel="'+esc(p.id)+'"'+(s.errors?' checked':'')+'>'+esc(D.t('errorOnly'))+'</label><input class="argus-input argus-duration-input" type="number" min="0" data-viz-filter="min" data-viz-panel="'+esc(p.id)+'" value="'+s.min+'" aria-label="'+esc(D.t('minDuration'))+'">':'<select class="argus-select" data-viz-filter="level" data-viz-panel="'+esc(p.id)+'" aria-label="'+esc(D.t('level'))+'">'+['All','ERROR','WARN','INFO'].map(v=>'<option'+(s.level===v?' selected':'')+'>'+v+'</option>').join('')+'</select>')+
      '<select class="argus-select" data-viz-filter="sort" data-viz-panel="'+esc(p.id)+'" aria-label="'+esc(D.t('sort'))+'"><option value="newest"'+(s.sort==='newest'?' selected':'')+'>'+esc(D.t('newest'))+'</option><option value="oldest"'+(s.sort==='oldest'?' selected':'')+'>'+esc(D.t('oldest'))+'</option></select>'+
      '<button class="argus-button argus-button--small" data-viz-action="clear" data-viz-panel="'+esc(p.id)+'">'+esc(D.t('clearFilters'))+'</button></div>';
  }
  function labels(r,p){
    if(D.sources?.enabled(p))return ['service','environment','resource','severity'].map(k=>'<span class="argus-badge">'+esc(k+'='+r[k])+'</span>').join('');
    return ['service','environment','resource','severity'].map(k=>'<button class="argus-label-filter" data-viz-action="label" data-viz-panel="'+esc(p.id)+'" data-key="'+k+'" data-value="'+esc(r[k])+'">'+esc(k)+'='+esc(r[k])+'</button>').join('');
  }
  function logView(p,result,expanded){
    const rows=D.sources?.enabled(p)?result.rows:filterRows(p.id,result.rows),s=D.sources?.enabled(p)?D.sources.filterState(p,D.ui.ctx):state(p.id);
    return '<div class="argus-log-view">'+tools(p)+
      (expanded?'<div class="argus-log-volume" data-log-volume="'+esc(p.id)+'" aria-label="'+esc(D.t('logVolume'))+'"></div>':'')+
      '<div class="argus-row"><small class="argus-muted">'+esc(D.t('rows',{n:rows.length}))+' / '+result.rows.length+'</small>'+Object.entries(s.labels||{}).map(([k,v])=>'<span class="argus-badge">'+esc(k+'='+v)+'</span>').join('')+'</div>'+
      '<div class="argus-log-lines">'+rows.map(r=>'<details class="argus-log-line"><summary><span class="argus-log-time">−'+r.age+'m</span><span class="argus-level argus-level--'+r.severity.toLowerCase()+'">'+r.severity+'</span><span class="argus-log-body">'+highlight(r.body,s.keyword)+'</span></summary><div class="argus-stack"><div class="argus-row">'+labels(r,p)+'</div><pre>'+esc(JSON.stringify({timestamp:'−'+r.age+'m',body:r.body,attributes:{service_name:r.service,environment:r.environment,host:r.resource},trace_id:r.traceId||r.id},null,2))+'</pre><div class="argus-row"><button class="argus-button argus-button--small" data-viz-action="context" data-viz-panel="'+esc(p.id)+'" data-row="'+esc(r.id)+'">'+esc(D.t('contextLines'))+'</button><button class="argus-button argus-button--small" data-viz-action="trace" data-viz-panel="'+esc(p.id)+'" data-row="'+esc(r.id)+'">'+esc(D.t('viewTrace'))+'</button></div></div></details>').join('')+(!rows.length?'<div class="argus-empty">'+esc(D.t('noData'))+'</div>':'')+'</div></div>';
  }
  function traceList(p,result,expanded){
    if(D.sources?.enabled(p))return D.sourceViews.traceContent(p,result);
    const rows=filterRows(p.id,result.rows);
    return '<div class="argus-log-view">'+tools(p,true)+(expanded?'<div class="argus-trace-distribution" data-trace-distribution="'+esc(p.id)+'"></div>':'')+
      '<div class="argus-table-wrap argus-trace-table"><table class="argus-table"><thead><tr><th>Trace / '+esc(D.t('service'))+'</th><th>'+esc(D.t('durationLabel'))+'</th><th>'+esc(D.t('status'))+'</th><th></th></tr></thead><tbody>'+rows.map(r=>'<tr><td><strong>'+esc(r.service)+'</strong><br><small class="argus-mono">'+esc(r.id)+'</small></td><td>'+r.duration+' ms</td><td><span class="argus-level argus-level--'+(r.status==='ERROR'?'error':'info')+'">'+r.status+'</span></td><td><button class="argus-button argus-button--small" data-viz-action="trace" data-viz-panel="'+esc(p.id)+'" data-row="'+esc(r.id)+'">'+esc(D.t('waterfall'))+'</button></td></tr>').join('')+'</tbody></table></div></div>';
  }
  function spans(record){
    const duration=record.duration,err=record.status==='ERROR';
    return [
      ['s1','',record.service,record.operation||'HTTP POST /v1/payments',0,1,'SERVER',false],
      ['s2','s1','payment-api','payment.authorize',.035,.92,'SERVER',false],
      ['s3','s2','redis','GET session',.065,.045,'CLIENT',false],
      ['s4','s2','risk-provider','POST /risk/evaluate',.14,.62,'CLIENT',err],
      ['s5','s4','risk-provider','risk.evaluate',.16,.58,'SERVER',err],
      ['s6','s5','postgresql','SELECT risk_rules',.23,.46,'CLIENT',err],
      ['s7','s2','ledger','POST /ledger/entries',.8,.12,'CLIENT',false],
      ['s8','s7','postgresql','INSERT ledger_entries',.815,.09,'CLIENT',false]
    ].map(([id,parent,service,operation,start,ratio,kind,error])=>({id,parent,service,operation,start:Math.round(start*duration),duration:Math.round(ratio*duration),kind,error,tags:{'service.name':service,'span.kind':kind,'http.route':(record.operation||'POST /v1/payments').split(' ').at(-1),'resource.id':record.resource,...(record.provider==='skywalking'?{'sw8.segment_id':record.segmentId,'sw8.span_id':id}:record.provider==='jaeger'?{'process.serviceName':service,'opentracing.ref_type':parent?'child-of':'root'}:{})},events:error?[{time:Math.round((start+ratio)*duration),name:'exception',message:'upstream timeout'}]:[]}));
  }
  function waterfall(u){
    const all=spans(u.record),selected=all.find(s=>s.id===u.selected)||all[0],term=(u.search||'').toLowerCase(),collapsed=u.collapsed||[];
    function depth(s){let n=0,p=s.parent;while(p){n++;p=all.find(x=>x.id===p)?.parent}return n}
    function hidden(s){let p=s.parent;while(p){if(collapsed.includes(p))return true;p=all.find(x=>x.id===p)?.parent}return false}
    const visible=all.filter(s=>!hidden(s)&&(!term||(s.service+' '+s.operation).toLowerCase().includes(term)));
    return (D.sourceViews?.detailHeader(u.record)||'')+'<div class="argus-row argus-between"><div><strong>'+esc(u.record.service)+'</strong> <code>'+esc(u.record.id)+'</code></div><span class="argus-badge">'+u.record.duration+' ms · '+all.length+' spans</span></div>'+
      '<input class="argus-input" data-span-search value="'+esc(u.search||'')+'" placeholder="'+esc(D.t('spanSearch'))+'" aria-label="'+esc(D.t('spanSearch'))+'">'+
      '<div class="argus-waterfall-layout"><div class="argus-waterfall"><div class="argus-waterfall-scale"><span>Span</span><span>0 ms</span><span>'+u.record.duration+' ms</span></div>'+
      visible.map(s=>'<div class="argus-span-row'+(s.id===selected.id?' is-selected':'')+'"><div class="argus-span-name" style="padding-left:calc('+depth(s)+' * var(--space-3))"><button class="argus-span-toggle" data-viz-action="collapse-span" data-span="'+s.id+'" aria-label="'+esc(tr('展开或收起 ','Expand or collapse ')+s.operation)+'">'+(all.some(x=>x.parent===s.id)?collapsed.includes(s.id)?'▸':'▾':'·')+'</button><button class="argus-span-label" data-viz-action="select-span" data-span="'+s.id+'"><b>'+esc(s.service)+'</b><small>'+esc(s.operation)+'</small></button></div><button class="argus-span-track" data-viz-action="select-span" data-span="'+s.id+'" aria-label="'+esc(s.operation)+' '+s.duration+' ms"><i class="'+(s.error?'is-error':'')+'" style="left:'+s.start/u.record.duration*100+'%;width:'+Math.max(1,s.duration/u.record.duration*100)+'%"></i><span>'+s.duration+' ms</span></button></div>').join('')+'</div>'+
      '<aside class="argus-span-details"><h3>'+esc(D.t('spanDetails'))+'</h3><strong>'+esc(selected.operation)+'</strong><div class="argus-row"><span class="argus-badge">'+selected.kind+'</span><span class="argus-badge">'+selected.duration+' ms</span><span class="argus-badge">'+(selected.error?'ERROR':'OK')+'</span></div><h3>'+esc(D.t('attributes'))+'</h3>'+Object.entries(selected.tags).map(([k,v])=>'<div><small>'+esc(k)+'</small><div>'+esc(v)+'</div></div>').join('')+'<h3>'+esc(D.t('events'))+'</h3><pre>'+esc(JSON.stringify(selected.events,null,2))+'</pre></aside></div><small class="argus-muted">'+esc(D.t('backendPending'))+'</small>';
  }
  function isModal(kind){return ['viz-data','log-explorer','trace-explorer','trace-waterfall','log-context'].includes(kind)}
  function modal(u){
    const V=D.views;
    if(u.kind==='trace-waterfall')return V.modal(D.t('waterfall'),waterfall(u),V.button('close','modal-close'));
    if(u.kind==='log-context'){
      const target=u.result.rows.find(r=>r.id===u.row);const rows=u.result.rows.filter(r=>!target||(r.resource===target.resource&&r.service===target.service&&r.environment===target.environment)).sort((a,b)=>a.age-b.age),i=rows.findIndex(r=>r.id===u.row);
      return V.modal(D.t('contextLines'),'<small>'+esc(D.t('localFilters'))+'</small><pre>'+esc(rows.slice(Math.max(0,i-3),i+4).map(r=>(r.id===u.row?'▶ ':'  ')+'−'+r.age+'m '+r.severity+' '+r.body).join('\n'))+'</pre>',V.button('close','modal-close'));
    }
    const body=u.kind==='viz-data'?dataTable(u.panel,u.result):u.kind==='log-explorer'?logView(u.panel,u.result,true):traceList(u.panel,u.result,true);
    return V.modal(D.t(u.kind==='viz-data'?'viewData':u.kind==='log-explorer'?'logExplore':'traceExplore'),'<small class="argus-muted">'+esc(D.t(D.sources?.enabled(u.panel)?'sourceProof':'localFilters'))+'</small>'+(D.sources?.enabled(u.panel)?D.sourceViews.controls(u.panel):'')+body,V.button('close','modal-close'));
  }
  function panelOptions(p){
    const V=D.views,o=display(p),r=D.grid.normalize(p.layout||{w:6,h:8}),store={'data-store':'panel-layout'};
    const fields=V.field('positionX','x',r.x,'input',[],{...store,type:'number',min:0,max:9})+V.field('positionY','y',r.y,'input',[],{...store,type:'number',min:0})+V.field('widthCols','w',r.w,'input',[],{...store,type:'number',min:3,max:12})+V.field('heightRows','h',r.h,'input',[],{...store,type:'number',min:5,max:28});
    const opts=p.signal==='metrics'?V.field('decimals','decimals',o.decimals,'input',[],{'data-store':'panel-display',type:'number',min:0,max:6})+V.field('reducer','reducer',o.reducer,'select',[['last',tr('最后值','Last')],['mean',tr('平均值','Mean')],['min',tr('最小值','Min')],['max',tr('最大值','Max')],['sum',tr('总和','Sum')]],{'data-store':'panel-display'})+V.field('minValue','min',o.min,'input',[],{'data-store':'panel-display',type:'number'})+V.field('maxValue','max',o.max,'input',[],{'data-store':'panel-display',type:'number'})+V.field('lineStyle','style',o.style,'select',[['line',tr('折线','Line')],['area',tr('面积','Area')],['bar',tr('柱形','Bars')]],{'data-store':'panel-display'})+['legend','stack','smooth'].map((key,i)=>'<label class="argus-check"><input type="checkbox" data-display-flag="'+key+'"'+(o[key]?' checked':'')+'>'+esc(D.t(['showLegend','stacking','smooth'][i]))+'</label>').join(''):'';
    return '<details class="argus-detail" open><summary>'+esc(D.t('visualOptions'))+'</summary><div class="argus-form-grid">'+fields+opts+'</div></details>';
  }
  function currentPanel(id){const data=D.currentData();return data?.panels.find(p=>p.id===id)}
  function currentResult(p){const data=D.currentData();return M.panelData(p,D.ui.ctx,data.variables,D.ui.scene)}
  function openExplore(p){D.openModal({kind:p.signal==='metrics'||p.signal==='logs'&&M.effectiveBuilder(p)?.op!=='records'?'viz-data':p.signal==='logs'?'log-explorer':'trace-explorer',panel:p,result:currentResult(p)})}
  function trace(record){
    if(D.sources?.providers[record.provider]?.signal==='logs')record={...record,provider:record.traceProvider,id:record.traceId};
    D.openModal({kind:'trace-waterfall',record,selected:'s1',collapsed:[],search:''});
  }
  function attachChart(node,option,map,key){
    if(!window.echarts){node.textContent=tr('请先安装仓库依赖以加载 ECharts。','Install repository dependencies to load ECharts.');return}
    const chart=echarts.init(node,null,{renderer:'canvas'});chart.setOption(option);
    const observer=new ResizeObserver(()=>{if(!chart.isDisposed())chart.resize()});observer.observe(node);
    map.set(key,chart);observers.set(chart,observer);return chart;
  }
  function mount(root=document){
    root?.querySelectorAll('[data-echart]').forEach(node=>{
      const p=currentPanel(node.dataset.echart);if(!p)return;const option=configuration(p,currentResult(p));
      if(p.type==='stat'){option.legend.show=false;option.xAxis.show=false;option.yAxis.show=false;option.dataZoom=[];option.grid={left:0,right:0,top:4,bottom:0}}
      const chart=attachChart(node,option,charts,node.dataset.echart);if(chart&&p.type==='apm_topology')chart.on('click',params=>{if(params.dataType==='node')D.apm.openEntity(p,'service',params.name)});
    });
  }
  function disposeChart(chart){observers.get(chart)?.disconnect();observers.delete(chart);chart.dispose()}
  function dispose(){charts.forEach(disposeChart);charts.clear()}
  function disposePanel(id){const chart=charts.get(id);if(chart){disposeChart(chart);charts.delete(id)}}
  function disposeModal(){modalCharts.forEach(disposeChart);modalCharts=[]}
  function mountModal(){
    const u=D.ui.modal;if(!u||!['log-explorer','trace-explorer'].includes(u.kind))return;
    const node=document.querySelector('[data-log-volume],[data-trace-distribution]');if(!node||!window.echarts)return;
    const rows=D.sources?.enabled(u.panel)?u.result.rows:filterRows(u.panel.id,u.result.rows),c=colors(),map=new Map();
    const option=u.kind==='log-explorer'?{animation:false,tooltip:{trigger:'axis',confine:true},grid:{left:35,right:15,top:10,bottom:22},xAxis:{type:'category',data:Array.from({length:12},(_,i)=>(i*5)+'m'),axisLabel:{color:c.muted}},yAxis:{type:'value',axisLabel:{color:c.muted},splitLine:{lineStyle:{color:c.line}}},series:['ERROR','WARN','INFO'].map((level,i)=>({name:level,type:'bar',stack:'logs',itemStyle:{color:[c.palette[4],c.palette[3],c.palette[0]][i]},data:Array.from({length:12},(_,bucket)=>rows.filter(r=>r.severity===level&&Math.floor(r.age/5)===bucket).length)}))}:{animation:false,tooltip:{trigger:'item',confine:true},grid:{left:48,right:15,top:10,bottom:22},xAxis:{type:'value',name:'min',axisLabel:{color:c.muted}},yAxis:{type:'value',name:'ms',axisLabel:{color:c.muted},splitLine:{lineStyle:{color:c.line}}},series:[{type:'scatter',symbolSize:9,data:rows.map(r=>({value:[-r.age,r.duration],record:r,itemStyle:{color:r.status==='ERROR'?c.palette[4]:c.palette[0]}}))}]};
    const chart=attachChart(node,option,map,'modal');if(chart){modalCharts.push(chart);if(u.kind==='trace-explorer')chart.on('click',params=>{if(params.data.record)trace(params.data.record)})}
  }
  function tryChange(id){
    if(!['rich-metrics','rich-logs','rich-traces','free-layout'].includes(id))return false;
    if(!M.actor().manage&&id==='free-layout'){D.state.actor='editor';M.persist()}
    upgrade(D.state);
    if(id==='free-layout'){D.editBoard('showcase');document.querySelector('[data-dashboard-grid]')?.scrollIntoView({block:'start'})}else D.openBoard('showcase');
    if(id==='rich-logs'||id==='rich-traces'){const p=currentPanel(id==='rich-logs'?'rich-logs':'rich-traces');openExplore(p)}
    return true;
  }
  document.addEventListener('click',event=>{
    const el=event.target.closest('[data-viz-action],[data-action=\"viz-data\"],[data-action=\"viz-explore\"]');if(!el)return;
    if(el.closest('#app')&&D.ui.modal)return;
    const action=el.dataset.vizAction||el.dataset.action,p=el.dataset.vizPanel?currentPanel(el.dataset.vizPanel):currentPanel(el.dataset.id);
    if(action==='viz-data'||action==='viz-explore'){if(p)openExplore(p);return}
    const u=D.ui.modal;
    if(action==='select-span'){u.selected=el.dataset.span;D.renderModal(false);return}
    if(action==='collapse-span'){const set=new Set(u.collapsed);set.has(el.dataset.span)?set.delete(el.dataset.span):set.add(el.dataset.span);u.collapsed=[...set];D.renderModal(false);return}
    if(!p)return;
    if(action==='clear'){D.ui.vizFilters[p.id]={keyword:'',level:'All',sort:'newest',wrap:true,labels:{},min:0,errors:false}}
    if(action==='label')state(p.id).labels[el.dataset.key]=el.dataset.value;
    if(['context','trace'].includes(action)&&D.apm&&!D.apm.allowed(p,action==='context'?'log_context':'trace_detail')){D.toast(D.t('drilldownMissing'));return}
    if(action==='context'){D.openModal({kind:'log-context',panel:p,result:currentResult(p),row:el.dataset.row});return}
    if(action==='trace'){const r=currentResult(p).rows.find(r=>r.id===el.dataset.row);if(r){const result=D.apm?D.apm.traceTarget(p,r,D.ui.ctx,D.ui.scene):{record:r};if(result.error)D.toast(D.t(result.error));else trace(result.record)}return}
    if(u)D.renderModal(false);else D.render();
  });
  let searchTimer;
  document.addEventListener('input',event=>{
    const el=event.target;
    if(el.hasAttribute('data-span-search')){
      D.ui.modal.search=el.value;clearTimeout(searchTimer);searchTimer=setTimeout(()=>{const pos=el.selectionStart;D.renderModal(false);const input=document.querySelector('[data-span-search]');input?.focus();input?.setSelectionRange(pos,pos)},180);return;
    }
    if(el.dataset.vizFilter==='keyword'){
      state(el.dataset.vizPanel).keyword=el.value;clearTimeout(searchTimer);const id=el.dataset.vizPanel;
      searchTimer=setTimeout(()=>{const pos=el.selectionStart;if(D.ui.modal)D.renderModal(false);else D.render();const input=document.querySelector('[data-viz-filter=\"keyword\"][data-viz-panel=\"'+id+'\"]');input?.focus({preventScroll:true});input?.setSelectionRange(pos,pos)},180);
    }
  });
  document.addEventListener('change',event=>{
    const el=event.target;
    if(el.dataset.vizFilter&&el.dataset.vizFilter!=='keyword'){
      state(el.dataset.vizPanel)[el.dataset.vizFilter]=el.type==='checkbox'?el.checked:el.value;
      if(D.ui.modal)D.renderModal(false);else D.render();
    }
    if(el.dataset.store==='panel-layout'){const p=D.ui.modal.panel;p.layout=D.grid.normalize({...p.layout,[el.dataset.field]:Number(el.value)});D.renderModal(false)}
    if(el.dataset.store==='panel-display'){const p=D.ui.modal.panel;p.display={...display(p),[el.dataset.field]:['decimals','min','max'].includes(el.dataset.field)?Number(el.value):el.value};p.display.decimals=Math.max(0,Math.min(6,p.display.decimals));if(p.display.max<=p.display.min)p.display.max=p.display.min+1}
    if(el.dataset.displayFlag){const p=D.ui.modal.panel;p.display={...display(p),[el.dataset.displayFlag]:el.checked}}
  });
  D.visuals={spans,upgrade,content,panelOptions,isModal,modal,mount,mountModal,dispose,disposePanel,disposeModal,tryChange,trace,configuration};
})();
