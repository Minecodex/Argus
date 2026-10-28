import assert from 'node:assert/strict';
import {mkdtemp,writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {chromium} from '@playwright/test';
const output=await mkdtemp(join(tmpdir(),'argus-plan10-v3-'));
const browser=await chromium.launch();
const page=await browser.newPage({viewport:{width:1536,height:1080}});
const errors=[],passed=[];
page.on('pageerror',e=>errors.push(e.message));
page.on('requestfailed',r=>errors.push(r.url()));
await page.addInitScript(()=>{window.storageAccess=[];for(const method of ['setItem','getItem']){const original=Storage.prototype[method];Storage.prototype[method]=function(...args){window.storageAccess.push([method,args[0]]);return original.apply(this,args);};}});
const reset=()=>page.goto(new URL('./demo.html',import.meta.url).href);
const panels=()=>page.locator('[data-panel]'), first=()=>panels().first(), last=()=>panels().last();
const act=(root,name,value)=>root.locator(`[data-act="${name}"]${value?`[data-value="${value}"]`:''}`);
const mode=(root,value)=>act(root,'authoring',value).click();
const field=(root,key)=>root.locator(`[data-builder="${key}"]`);
const waitReady=()=>page.waitForFunction(()=>![...document.querySelectorAll('[data-status]')].some(e=>/正在|Loading/.test(e.textContent)));
const run=async root=>{await act(root,'run').click();await waitReady();};
const add=async signal=>{await act(page,'add').first().click();if(signal!=='metrics')await last().locator('[data-control="panel-signal"]').selectOption(signal);};
const resourceKind={ 'demo-host-b04':'host','demo-host-b05':'host','demo-cluster':'kubernetes_cluster','demo-namespace':'kubernetes_namespace','demo-application':'application','demo-redis':'monitoring_target' };
const chooseScope=async(root,ids)=>{
 await act(root,'panel-scope').click();await act(page,'scope-clear').click();
 for(const id of ids){await act(page,'scope-kind',resourceKind[id]).click();await page.getByRole('dialog').locator(`tbody [data-act="scope-toggle"][data-value="${id}"]`).click();}
 await act(page,'scope-confirm').click();
};
const tab=value=>act(page,'tab',value).click();
const check=async(name,fn)=>{await fn();passed.push(name);};
const screenshot=async name=>{await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));await page.screenshot({path:join(output,name),fullPage:true});};
try{
 await reset();
 await check('single-menu-default-builders-no-save',async()=>{
  assert.equal(await page.locator('h1').textContent(),'观测查询');
  assert.equal(await page.locator('.argus-sidebar [aria-current="page"]').count(),1);
  assert.equal(await page.locator('.argus-sidebar [data-act="signal"]').count(),0);
  assert.equal(await panels().count(),2);assert.equal(await page.locator('.argus-builder-grid').count(),2);
  assert.equal(await page.getByRole('button',{name:/保存|Save/}).count(),0);
  assert.equal(await page.getByRole('tab').count(),2);
  await screenshot('builders-light.png');
 });
 await check('metric-builder-filter-calculation-grouping',async()=>{
  await field(first(),'metric').selectOption('system_memory_utilization');
  await field(first(),'operation').selectOption('avg');
  await chooseScope(first(),['demo-host-b04']);
  await field(first(),'group').selectOption('argus_resource_id');
  assert.match(await first().locator('.argus-generated pre').textContent(),/avg by \(argus_resource_id\)/);
  await run(first());await act(first(),'table').click();assert.equal(await first().locator('tbody tr').count(),1);
  assert.match(await first().locator('tbody').textContent(),/B04-Cache-Collector/);
 });
 await check('metric-builder-lossless-round-trip',async()=>{
  const before=await first().locator('.argus-generated pre').textContent();
  await mode(first(),'dsl');assert.equal(await first().locator('textarea').inputValue(),before);
  await mode(first(),'builder');assert.match(await act(first(),'panel-scope').textContent(),/B04-Cache-Collector/);
  assert.equal(await first().locator('.argus-generated pre').textContent(),before);
 });
 await check('gauge-does-not-offer-rate-counter-does',async()=>{
  assert.equal(await field(first(),'operation').locator('option[value="rate"]').count(),0);
  await field(first(),'metric').selectOption('system_network_io');await field(first(),'operation').selectOption('rate');
  assert.match(await first().locator('.argus-generated pre').textContent(),/rate\(system_network_io\[5m\]\)/);
  await run(first());assert.match(await first().locator('[data-result]').textContent(),/KiB\/s/);
 });
 await check('logs-builder-filters-real-fixture-rows',async()=>{
  await add('logs');await field(last(),'level').selectOption('ERROR');await field(last(),'keyword').fill('Redis');await field(last(),'limit').selectOption('1');
  await run(last());assert.equal(await last().locator('tbody tr').count(),1);
  assert.match(await last().locator('tbody').textContent(),/Redis request timed out/);
 });
 await check('logs-builder-code-round-trip-and-count',async()=>{
  const before=await last().locator('.argus-generated pre').textContent();await mode(last(),'dsl');assert.equal(await last().locator('textarea').inputValue(),before);
  await mode(last(),'builder');assert.equal(await field(last(),'keyword').inputValue(),'Redis');
  await field(last(),'output').selectOption('count');await run(last());assert.match(await last().locator('table').textContent(),/匹配记录数/);assert.equal(await last().locator('tbody td').first().textContent(),'1');
 });
 await check('traces-builder-status-duration-trace-id',async()=>{
  await add('traces');await field(last(),'status').selectOption('ERROR');await field(last(),'duration').fill('1000');await run(last());
  assert.equal(await last().locator('tbody tr').count(),1);assert.match(await last().locator('tbody').textContent(),/3210 ms/);
  await field(last(),'trace').fill('missing-trace');await run(last());assert.match(await last().locator('[data-result]').textContent(),/没有示例记录/);
  await field(last(),'trace').fill('');await run(last());
 });
 await check('traces-builder-lossless-round-trip',async()=>{
  const before=await last().locator('.argus-generated pre').textContent();await mode(last(),'dsl');assert.equal(await last().locator('textarea').inputValue(),before);
  await mode(last(),'builder');assert.equal(await field(last(),'duration').inputValue(),'1000');assert.equal(await field(last(),'status').inputValue(),'ERROR');
 });
 await check('mixed-signals-run-all-and-dark-english',async()=>{
  await act(page,'run-all').click();await waitReady();
  assert.equal(await page.locator('[data-status]').filter({hasText:'示例结果'}).count(),4);
  await screenshot('mixed-signals.png');
  await act(page,'theme').click();await act(page,'lang').click();assert.equal(await page.locator('h1').textContent(),'Observability queries');
  await screenshot('mixed-dark-en.png');await act(page,'lang').click();await act(page,'theme').click();
 });
 await check('all-three-reject-lossy-reverse-conversion',async()=>{
  for(const index of [0,2,3]){
   const p=panels().nth(index);await mode(p,'dsl');const text=(await p.locator('textarea').inputValue())+' # unsupported prototype edit';
   await p.locator('textarea').fill(text);await mode(p,'builder');assert.equal(await p.locator('textarea').inputValue(),text);
   assert.match(await p.getByRole('alert').textContent(),/无法无损/);
  }
 });
 await check('signal-switch-retains-only-in-memory-draft',async()=>{
  const before=await first().locator('textarea').inputValue();await first().locator('[data-control="panel-signal"]').selectOption('logs');
  assert.equal(await first().locator('.argus-builder-grid').count(),1);
  await first().locator('[data-control="panel-signal"]').selectOption('metrics');assert.equal(await first().locator('textarea').inputValue(),before);
 });
 await check('reload-discards-temporary-panels-no-storage-access',async()=>{
  assert.deepEqual(await page.evaluate(()=>window.storageAccess),[]);
  await reset();assert.equal(await panels().count(),2);assert.equal(await page.locator('textarea').count(),0);
  assert.equal(await field(first(),'metric').inputValue(),'system_cpu_utilization');assert.deepEqual(await page.evaluate(()=>window.storageAccess),[]);
 });
 await check('metric-label-value-function-duration-completion',async()=>{
  await mode(first(),'dsl');await act(first(),'try-complete').click();let ed=first().locator('textarea');
  await ed.press('ArrowDown');await ed.press('Enter');assert.equal(await ed.inputValue(),'system_memory_utilization');
  await ed.fill('system_cpu_utilization{sta');await ed.press('Tab');await ed.press('Enter');assert.equal(await ed.inputValue(),'system_cpu_utilization{state="idle"');
  await ed.fill('ra');await ed.press('Enter');assert.equal(await ed.inputValue(),'rate(');
  await ed.fill('rate(system_network_io[');await ed.press('ArrowDown');await ed.press('Enter');assert.equal(await ed.inputValue(),'rate(system_network_io[5m');
 });
 await check('unknown-code-never-fakes-success',async()=>{
  await first().locator('textarea').fill('unknown_expression');await first().locator('textarea').press('Shift+Enter');await waitReady();assert.match(await first().locator('[data-result]').textContent(),/不执行任意查询/);
 });
 await reset();
 await check('catalog-all-signals-preserves-workspace',async()=>{
  await tab('catalog');await page.locator('[data-control="search"]').fill('redis');
  await act(page,'metric-detail','redis_memory_used').click();assert.equal(await page.getByRole('dialog').count(),1);
  await act(page.getByRole('dialog'),'use-metric').click();await waitReady();assert.equal(await panels().count(),3);assert.equal(await last().locator('.argus-builder-grid').count(),1);
  await tab('catalog');await act(page,'catalog-signal','logs').click();assert.match(await page.locator('#tab-content').textContent(),/service_name/);
  await act(page,'use-signal-example').click();assert.equal(await panels().count(),4);assert.equal(await last().locator('[data-control="panel-signal"]').inputValue(),'logs');
  await tab('catalog');await act(page,'catalog-signal','traces').click();assert.match(await page.locator('#tab-content').textContent(),/queryTraces/);
 });
 await check('unobserved-metric-stays-empty',async()=>{
  await act(page,'catalog-signal','metrics').click();await page.locator('[data-control="observed"]').uncheck();await act(page,'use-metric','redis_memory_rss').click();await waitReady();
  assert.match(await last().locator('[data-result]').textContent(),/未观测到/);
 });
 await reset();
 await check('duplicate-builder-independence-reorder-remove',async()=>{
  await act(first(),'duplicate').click();await field(panels().nth(1),'metric').selectOption('redis_memory_used');assert.equal(await field(first(),'metric').inputValue(),'system_cpu_utilization');
  await act(panels().nth(1),'move-up').click();assert.equal(await field(first(),'metric').inputValue(),'redis_memory_used');await act(first(),'remove').click();assert.equal(await panels().count(),2);
 });
 await check('scope-time-invalidation-and-cancellation',async()=>{
  await chooseScope(first(),['demo-host-b04']);await page.locator('[data-control="range"]').selectOption('15');assert.equal(await page.locator('.argus-chart').count(),0);
  await act(page,'run-all').click();await waitReady();assert.equal(await page.locator('.argus-chart').count(),2);assert.match(await first().locator('.argus-chart').textContent(),/10:45/);
  await act(first(),'run').click();await act(first(),'cancel').click();await page.waitForTimeout(500);assert.match(await first().locator('[data-status]').textContent(),/已取消/);
 });
 await reset();
 await check('linked-cursor-instant-table-and-options',async()=>{
  await page.locator('.argus-chart').first().hover({position:{x:450,y:60}});const xs=await page.locator('.argus-crosshair').evaluateAll(es=>es.map(e=>e.getAttribute('x1')));assert.equal(xs[0],xs[1]);
  await act(first(),'advanced').click();await first().locator('[data-control="mode"]').selectOption('instant');await first().locator('[data-control="step"]').selectOption('30');await run(first());
  assert.equal(await first().locator('tbody tr').count(),2);assert.equal(await act(first(),'graph').isDisabled(),true);
 });
 await check('log-to-trace-keeps-existing-metric-panels',async()=>{
  await add('logs');await run(last());await act(last(),'expand-log').first().click();await act(last(),'related-trace').click();await waitReady();assert.equal(await panels().count(),4);
  assert.equal(await last().locator('tbody tr').count(),1);await act(last(),'trace-detail').click();assert.equal(await page.locator('.argus-waterfall').count(),3);await page.keyboard.press('Escape');
 });
 await check('resource-scope-does-not-leak-log-fixtures',async()=>{
  await chooseScope(last(),['demo-host-b04']);await act(page,'run-all').click();await waitReady();assert.match(await last().locator('[data-result]').textContent(),/没有示例记录/);
 });
 await check('keyboard-tabs-and-desktop-layout',async()=>{
  await page.locator('#tab-query').focus();await page.keyboard.press('ArrowRight');assert.equal(await page.locator('#tab-catalog').getAttribute('aria-selected'),'true');await page.keyboard.press('Home');
  for(const width of [1280,1440,1920]){await page.setViewportSize({width,height:900});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);}
 });
 await reset();
 await check('title-before-signal-and-no-global-scope',async()=>{
  assert.equal(await page.locator('.argus-toolbar [data-control="scope"]').count(),0);
  assert.equal(await page.locator('[data-builder="resource"]').count(),0);
  assert.equal(await first().evaluate(el=>Boolean(el.querySelector('.argus-panel-title').compareDocumentPosition(el.querySelector('[data-control="panel-signal"]'))&Node.DOCUMENT_POSITION_FOLLOWING)),true);
 });
 await check('scope-confirm-invalidates-only-target-panel',async()=>{
  const oldB=await panels().nth(1).locator('[data-result]').innerHTML();
  await chooseScope(first(),['demo-host-b04']);assert.equal(await first().locator('.argus-chart').count(),0);
  assert.equal(await panels().nth(1).locator('[data-result]').innerHTML(),oldB);
  await run(first());await act(first(),'table').click();assert.equal(await first().locator('tbody tr').count(),1);
  assert.match(await first().locator('tbody').textContent(),/B04-Cache-Collector/);
  await screenshot('panel-scopes-light.png');
 });
 await check('picker-search-kind-tabs-multiselect-and-cancel',async()=>{
  await act(first(),'panel-scope').click();await act(page,'scope-clear').click();
  await page.locator('[data-picker="search"]').fill('B05');assert.equal(await page.getByRole('dialog').locator('tbody tr').count(),1);
  await page.getByRole('dialog').locator('tbody [data-act="scope-toggle"]').click();
  await act(page,'scope-kind','kubernetes_cluster').click();await page.getByRole('dialog').locator('tbody [data-act="scope-toggle"]').click();
  assert.match(await page.locator('.argus-scope-selected').textContent(),/B05-Web-Collector/);assert.match(await page.locator('.argus-scope-selected').textContent(),/dev-cluster/);
  await screenshot('resource-picker.png');await page.getByRole('button',{name:'取消',exact:true}).click();assert.match(await act(first(),'panel-scope').textContent(),/B04-Cache-Collector/);
  assert.equal(await first().locator('tbody tr').count(),1);
 });
 await check('multi-resource-union-and-scope-survives-signal-switch',async()=>{
  await chooseScope(first(),['demo-host-b04','demo-host-b05','demo-cluster']);await run(first());assert.equal(await first().locator('tbody tr').count(),2);
  await first().locator('[data-control="panel-signal"]').selectOption('logs');assert.match(await act(first(),'panel-scope').textContent(),/3/);await run(first());assert.equal(await first().locator('tbody tr').count(),3);
  await first().locator('[data-control="panel-signal"]').selectOption('metrics');assert.match(await act(first(),'panel-scope').textContent(),/3/);
 });
 await check('explicit-empty-scope-never-means-all',async()=>{
  await chooseScope(first(),[]);await run(first());assert.match(await first().locator('[data-result]').textContent(),/未观测到/);assert.match(await act(first(),'panel-scope').textContent(),/0/);
  await act(first(),'panel-scope').click();await act(page,'scope-all').click();await act(page,'scope-confirm').click();await run(first());assert.equal(await first().locator('tbody tr').count(),2);
 });
 await check('catalog-scope-is-a-filter-and-does-not-touch-panels',async()=>{
  const before=await panels().nth(1).locator('[data-result]').innerHTML();await tab('catalog');
  assert.equal(await page.locator('.argus-filter [data-control="catalog-scope"]').count(),1);assert.equal(await page.locator('.argus-toolbar [data-control="catalog-scope"]').count(),0);
  await page.locator('[data-control="catalog-scope"]').selectOption('demo-host-b05');assert.equal(await page.locator('#tab-content tbody tr').count(),3);
  await page.locator('[data-control="group"]').selectOption('redis');assert.match(await page.locator('#tab-content tbody').textContent(),/没有匹配/);
  await page.locator('[data-control="group"]').selectOption('all');await screenshot('catalog-filter.png');await tab('query');
  assert.equal(await panels().nth(1).locator('[data-result]').innerHTML(),before);assert.match(await act(first(),'panel-scope').textContent(),/全部示例/);
 });
 await check('catalog-new-panel-inherits-only-catalog-scope',async()=>{
  await tab('catalog');await act(page,'use-metric','system_memory_utilization').click();await waitReady();
  assert.match(await act(last(),'panel-scope').textContent(),/B05-Web-Collector/);await act(last(),'table').click();assert.equal(await last().locator('tbody tr').count(),1);
  assert.match(await last().locator('tbody').textContent(),/B05-Web-Collector/);assert.match(await act(first(),'panel-scope').textContent(),/全部示例/);
 });
 await check('copied-panel-scope-is-independent',async()=>{
  const originalId=await last().getAttribute('data-panel');await act(last(),'duplicate').click();await chooseScope(last(),['demo-host-b04']);
  assert.match(await act(page.locator(`[data-panel="${originalId}"]`),'panel-scope').textContent(),/B05-Web-Collector/);
 });
 await check('resource-shortcut-adds-panel-without-changing-others',async()=>{
  const before=await first().locator('[data-result]').innerHTML(),count=await panels().count();await act(page,'cluster-entry').click();assert.equal(await panels().count(),count+1);
  assert.match(await act(last(),'panel-scope').textContent(),/dev-cluster/);assert.equal(await first().locator('[data-result]').innerHTML(),before);
 });
 await check('picker-keyboard-focus-and-dark-english',async()=>{
  await act(page,'theme').click();await act(page,'lang').click();await act(last(),'panel-scope').click();
  assert.match(await page.getByRole('dialog').textContent(),/Applies only to query/);await act(page,'scope-kind','host').focus();await page.keyboard.press('ArrowRight');assert.equal(await act(page,'scope-kind','kubernetes_cluster').getAttribute('aria-selected'),'true');
  await screenshot('resource-picker-dark-en.png');await page.keyboard.press('Escape');assert.equal(await page.getByRole('dialog').count(),0);
  assert.equal(await act(last(),'panel-scope').evaluate(el=>el===document.activeElement),true);
 });
 await check('no-browser-errors-and-no-persistence',async()=>{assert.deepEqual(errors,[]);assert.deepEqual(await page.evaluate(()=>window.storageAccess),[]);});
 await writeFile(join(output,'results.json'),JSON.stringify({scope:'v3 local HTML prototype, no backend acceptance',passed:passed.length,checks:passed,errors},null,2));
 console.log(JSON.stringify({passed:passed.length,checks:passed,output},null,2));
}catch(e){await screenshot('failure.png');console.error(JSON.stringify({passed,output,errors,failure:e.stack},null,2));process.exitCode=1;}finally{await browser.close();}
