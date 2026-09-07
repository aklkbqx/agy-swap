import test from 'node:test';
import assert from 'node:assert/strict';
import { build } from 'esbuild';
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { decodeFixtureDocument } from '../src/components/fixtureCodec.js';
const require = createRequire(import.meta.url);
const root = fileURLToPath(new URL('..', import.meta.url));

test('real demo preserves manual navigation and cancels stale view/layout loads', async () => {
 const dir=mkdtempSync(join(tmpdir(),'agy-demo-test-'));
 const constants=readFileSync(join(root,'src/components/fixtures.js'),'utf8').split('let currentGlobalFixture')[0].replace(/^import .*;\n/gm,'');
 const result=await build({entryPoints:[join(root,'src/components/InteractiveDemo.jsx')],bundle:true,write:false,platform:'node',format:'cjs',logLevel:'silent',plugins:[{name:'render-boundaries',setup(b){
  b.onResolve({filter:/^react$/},()=>({path:require.resolve('react'),external:true}));
  for(const [pattern,name]of [[/\.module\.css$/,'css'],[/\/I18nContext$/,'i18n'],[/^\.\/TuiTerminal$/,'terminal'],[/^\.\/(AmbientDust|TerminalScene)$/,'noop'],[/tui-initial-fixtures\.json$/,'initial'],[/^\.\/fixtures\.js$/,'fixtures']])b.onResolve({filter:pattern},()=>({path:name,namespace:'test'}));
  b.onLoad({filter:/.*/,namespace:'test'},({path})=>({loader:'js',contents:{
   css:'export default {};',i18n:'export const useTranslation=()=>({t:(key,fallback)=>fallback});',noop:'export default ()=>null;',initial:'export default {fixtures:[{view:"Dashboard"}]};',
   terminal:'import React from "react"; export default function Terminal(p){globalThis.demoAperture=p.onApertureCapacity; return React.createElement("div",{id:"terminal",tabIndex:0,onKeyDown:p.onKeyDown,"data-layout":p.fixture.layout},p.fixture.view);}',
   fixtures:constants+'\nexport const fetchShard=(layout,view)=>globalThis.demoFetch(layout,view); export const setGlobalTuiFixture=()=>{}; export const resolveFixture=(fixtures,state)=>({view:state.view,layout:state.layout});'
  }[path]}));
 }}]});
 const entry=join(dir,'demo.cjs');writeFileSync(entry,result.outputFiles[0].text);
 const dom=new JSDOM('<div id="root"></div>',{url:'http://localhost',pretendToBeVisual:true});
 const original=new Map();
 for(const [key,value]of Object.entries({window:dom.window,document:dom.window.document,HTMLElement:dom.window.HTMLElement,requestAnimationFrame:dom.window.requestAnimationFrame.bind(dom.window),cancelAnimationFrame:dom.window.cancelAnimationFrame.bind(dom.window),IS_REACT_ACT_ENVIRONMENT:true})){original.set(key,Object.getOwnPropertyDescriptor(globalThis,key));Object.defineProperty(globalThis,key,{value,writable:true,configurable:true});}
 let heldView,heldLayout;
 globalThis.demoFetch=(layout,view)=>{if(view==='Profiles')return new Promise(resolve=>{heldView=()=>resolve([{view,layout}]);});if(layout==='stacked')return new Promise(resolve=>{heldLayout=()=>resolve([{view,layout}]);});return Promise.resolve([{view,layout}]);};
 const mount=createRoot(document.getElementById('root'));
 const press=async key=>{await act(async()=>{document.getElementById('terminal').dispatchEvent(new dom.window.KeyboardEvent('keydown',{key,bubbles:true}));});};
 try{
  const Demo=require(entry).default;await act(async()=>mount.render(React.createElement(Demo,{is3DEnabled:false})));
  await press('v');assert.equal(document.getElementById('terminal').textContent,'Quota');
  await press('g');assert.equal(document.getElementById('terminal').textContent,'Dashboard');
  await press('p');assert.ok(heldView);await press('v');await act(async()=>heldView());
  assert.equal(document.getElementById('terminal').textContent,'Quota','cached navigation must cancel an older load');
  await act(async()=>globalThis.demoAperture({cols:80,rows:24}));assert.ok(heldLayout);
  await act(async()=>globalThis.demoAperture({cols:40,rows:14}));await act(async()=>heldLayout());
  assert.equal(document.getElementById('terminal').dataset.layout,'compact','late layout load must not undo newer capacity');
 }finally{
  await act(async()=>mount.unmount());dom.window.close();rmSync(dir,{recursive:true,force:true});
  for(const [key,descriptor]of original){if(descriptor)Object.defineProperty(globalThis,key,descriptor);else delete globalThis[key];}delete globalThis.demoFetch;delete globalThis.demoAperture;
 }
});
test('packed fixtures share row arrays and reject corrupt references',()=>{
 const packed={schema:2,version:'test',rows:['row'],frames:[{lines:[0],plain:[0]}],fixtures:[['a',0],['b',0]]};
 const {fixtures}=decodeFixtureDocument(packed);assert.deepEqual(fixtures[0].lines,['row']);assert.equal(fixtures[0].lines,fixtures[1].lines);
 assert.throws(()=>decodeFixtureDocument({...packed,fixtures:[['bad',99]]}));assert.throws(()=>decodeFixtureDocument({...packed,rows:[]}));
});
