import { chromium } from 'playwright';
import fs from 'fs';
const OUT = process.env.OUT || '/workspace/bluntcode-shots/audit3';
const base = process.env.BASE || 'http://127.0.0.1:8787', empty = process.env.EMPTY || 'http://127.0.0.1:8788';
const DEMO='de3ed857-8839-2317-d899-335a98e2f2af', DEEP='98466790-5531-aeb3-fd40-2119fcf5c49d', NEV='21cd7f79-8d3a-93d6-0e39-9288fbc3213f';
const S1='d5453e1f-9b18-8776-b17f-eb2aa699b9d0', SF='cdafa410-2e0b-c4dd-4604-39926dbc00ad', SOLD='314dc41e-18d0-83e1-f0ca-aab973f6d232', SD='1b8ec9ce-f536-4e8e-f990-76076592a4b3';
const VPS = { phone:[375,812], tablet:[768,1024], laptop:[1280,800], wide:[1920,1080], d1280:[1280,800], d1440:[1440,900], d1920:[1920,1080] };
const only = process.env.VP ? process.env.VP.split(',') : Object.keys(VPS);
const clickRow = async (p) => { await p.locator('table tbody tr').first().click(); await p.waitForTimeout(700); };
const openNav = async (p) => { const b = p.getByRole('button', { name: /Open navigation/ }); if (await b.isVisible().catch(()=>false)) { await b.click(); await p.waitForTimeout(300);} };
const V = [
 ['home','/'], ['workspaces','/workspaces'], ['ws-demo',`/workspaces/${DEMO}`], ['ws-deepsrc',`/workspaces/${DEEP}`], ['ws-never',`/workspaces/${NEV}`],
 ['ws-tab-trends',`/workspaces/${DEMO}`, async p=>{await p.getByRole('tab',{name:'Trends'}).click();}],
 ['ws-tab-severity',`/workspaces/${DEMO}`, async p=>{await p.getByRole('tab',{name:'Severity'}).click();}],
 ['ws-tab-languages',`/workspaces/${DEMO}`, async p=>{await p.getByRole('tab',{name:'Languages'}).click();}],
 ['ws-tab-compliance',`/workspaces/${DEMO}`, async p=>{await p.getByRole('tab',{name:'Compliance'}).click();}],
 ['files',`/workspaces/${DEMO}/files`], ['files-expanded',`/workspaces/${DEEP}/files`, async p=>{ for (let i=0;i<3;i++) await p.locator('[aria-expanded="false"]').nth(0).click({timeout:2000}).catch(()=>{}); }],
 ['history',`/workspaces/${DEMO}/scans`], ['history-compare',`/workspaces/${DEMO}/scans?compare=${SOLD}&with=${S1}`],
 ['scan',`/scans/${S1}`], ['scan-deepsrc',`/scans/${SD}`], ['scan-failed',`/scans/${SF}`], ['scan-missing','/scans/00000000-0000-0000-0000-000000000000'],
 ['scan-filtered',`/scans/${S1}?severity=high`],
 ['finding-detail',`/scans/${S1}`, clickRow],
 ['finding-detail-next',`/scans/${S1}`, async p=>{ await clickRow(p); await p.getByRole('button',{name:/next/i}).first().click({timeout:2000}).catch(()=>{}); }],
 ['row-menu',`/scans/${S1}`, async p=>{ await p.getByRole('button',{name:/^Actions for/}).first().click(); }],
 ['suppress-dialog',`/scans/${S1}`, async p=>{ await p.getByRole('button',{name:/^Actions for/}).first().click(); await p.getByRole('menuitem',{name:/Suppress/}).click(); }],
 ['bulk-select',`/scans/${S1}`, async p=>{ const c=p.locator('tbody input[type=checkbox]'); await c.nth(0).check(); await c.nth(1).check(); await p.evaluate(()=>scrollTo(0,0)); }],
 ['export-dialog',`/scans/${S1}`, async p=>{ await p.getByRole('button',{name:'Export report'}).click(); }],
 ['search','/search'], ['search-q','/search?q=secret'],
 ['search-advanced','/search?q=secret', async p=>{ await p.getByRole('button',{name:/Advanced/}).click(); }],
 ['tools','/tools'], ['tool-menu','/tools', async p=>{ await p.getByRole('button',{name:/^Actions for/}).first().click(); }],
 ['pentest','/pentest'], ['pentest-ws',`/workspaces/${DEMO}/pentest`],
 ['settings','/settings'], ['about','/about'], ['cli','/cli'], ['not-found','/nope'], ['rules-hidden','/rules'],
 ['add-workspace','/workspaces', async p=>{ await p.getByRole('button',{name:/Add workspace/}).first().click(); }],
 ['add-workspace-error','/workspaces', async p=>{ await p.getByRole('button',{name:/Add workspace/}).first().click(); const d=p.getByRole('dialog'); await d.getByLabel(/Folder path/).fill('/does/not/exist'); await d.getByRole('button',{name:/^Add workspace$/}).click(); await p.waitForTimeout(800); }],
 ['run-scan-dialog',`/workspaces/${NEV}`, async p=>{ await p.getByRole('button',{name:/^Run scan$/}).first().click(); }],
 ['ws-remove-menu','/workspaces', async p=>{ await p.getByRole('button',{name:/^Actions for never/}).click(); }],
 ['palette','/', async p=>{ await p.keyboard.press('Control+k'); }],
 ['shortcuts','/', async p=>{ await p.keyboard.press('Shift+?'); }],
 ['notifications','/', async p=>{ await openNav(p); await p.getByRole('button',{name:/Notifications/}).first().click(); }],
 ['nav-drawer','/', async p=>{ await openNav(p); }],
 ['empty-home','/', null, empty], ['empty-workspaces','/workspaces', null, empty], ['empty-search','/search', null, empty],
 ['error500-home','/', async p=>{}, base, true], ['error500-scan',`/scans/${S1}`, async p=>{}, base, true],
];
const b = await chromium.launch(); const log = [];
for (const vn of only) { const [w,h] = VPS[vn]; fs.mkdirSync(`${OUT}/${vn}`, { recursive: true });
  const ctx = await b.newContext({ viewport:{width:w,height:h}, reducedMotion:'reduce', hasTouch: w<1000, isMobile: w<500 });
  await ctx.addInitScript(() => localStorage.setItem('bluntcode-theme','light'));
  const names = process.env.ONLY ? process.env.ONLY.split(',') : null;
  for (const [name, path, act, host = base, err500] of V) { if (names && !names.includes(name)) continue; const p = await ctx.newPage(); const errs=[];
    p.on('console', m=>{ if (m.type()==='error') errs.push(m.text().slice(0,160)); }); p.on('pageerror', e=>errs.push('pageerror '+String(e).slice(0,160)));
    if (err500) await p.route('**/api/v1/**', r=>r.fulfill({status:500,contentType:'application/json',body:'{"error":{"code":"INTERNAL","message":"database is locked"}}'}));
    let note='ok';
    try { await p.goto(host+path,{waitUntil:'networkidle',timeout:30000}); await p.waitForTimeout(500); if (act) { await act(p); await p.waitForTimeout(600); } } catch(e){ note='ERR '+e.message.split('\n')[0].slice(0,140); }
    const m = await p.evaluate((isTouch) => {
      const de=document.documentElement; const vis = e => { const r=e.getBoundingClientRect(); const s=getComputedStyle(e); return r.width>0&&r.height>0&&s.visibility!=='hidden'&&s.display!=='none'; };
      const wide=[...document.querySelectorAll('body *')].filter(e=>{const r=e.getBoundingClientRect(); return r.right>innerWidth+1&&r.width>0&&vis(e)&&!e.closest('.table-scroll,.overflow-x-auto,pre,.chip-rail,.chip-scroller,[data-scroll-x]')&&!(()=>{let a=e.parentElement;while(a){const o=getComputedStyle(a).overflowX;if(o==='auto'||o==='scroll'||o==='hidden')return true;a=a.parentElement;}return false;})();}).slice(0,6).map(e=>e.tagName.toLowerCase()+'.'+String(e.className).split(' ').slice(0,2).join('.')+' r='+Math.round(e.getBoundingClientRect().right));
      const small=[...document.querySelectorAll('a,button,input,select,[role=button],[role=tab],[role=menuitem],summary')].filter(e=>vis(e)&&!(e.type==='hidden')).map(e=>{const r=e.getBoundingClientRect(); return {t:(e.getAttribute('aria-label')||e.innerText||e.tagName).trim().slice(0,28),w:Math.round(r.width),h:Math.round(r.height)};}).filter(x=>x.h<40||x.w<40);
      const clipped=[...document.querySelectorAll('body *')].filter(e=>{ if(!vis(e)||!e.childNodes.length) return false; const s=getComputedStyle(e); if(!['hidden','clip'].includes(s.overflowX)) return false; return e.scrollWidth>e.clientWidth+2 && s.textOverflow!=='ellipsis' && ![...e.children].some(c=>getComputedStyle(c).textOverflow==='ellipsis') && e.innerText.trim().length>0; }).slice(0,6).map(e=>e.tagName.toLowerCase()+'.'+String(e.className).split(' ').slice(0,2).join('.')+' "'+e.innerText.trim().slice(0,30)+'"');
      const h1=[...document.querySelectorAll('h1')].map(e=>e.innerText.trim());
      const main=document.querySelector('main')||document.body; const words=(main.innerText||'').split(/\s+/).filter(Boolean).length;
      const paras=[...main.querySelectorAll('p,.eyebrow,.subtitle,.page-sub,.hint,.help,small,[class*=lede],[class*=description],[class*=muted]')].filter(vis).map(e=>e.innerText.trim()).filter(t=>t.split(/\s+/).length>=6);
      const fonts=new Set([...main.querySelectorAll('*')].filter(vis).filter(e=>[...e.childNodes].some(n=>n.nodeType===3&&n.textContent.trim())).map(e=>getComputedStyle(e).fontSize));
      return { sw:de.scrollWidth, iw:innerWidth, ox:de.scrollWidth-innerWidth, h:de.scrollHeight, wide, small:small.length, smallEx:small.slice(0,8), clipped, h1, words, paras:[...new Set(paras)].slice(0,25), fontSizes:[...fonts].sort() };
    }, w<1000).catch(e=>({err:String(e)}));
    await p.screenshot({ path:`${OUT}/${vn}/${name}.png`, fullPage: true }).catch(()=>{});
    await p.screenshot({ path:`${OUT}/${vn}/${name}--fold.png` }).catch(()=>{});
    log.push({ vp:vn, name, path, note, errs, ...m }); console.log(vn, name, note, 'ox='+m.ox, 'small='+m.small, 'words='+m.words, errs.length?'ERRS '+errs.length:'');
    await p.close(); }
  await ctx.close(); }
if (!process.env.ONLY) fs.writeFileSync(`${OUT}/audit3${process.env.VP?'-'+process.env.VP:''}.json`, JSON.stringify(log,null,1)); await b.close();
