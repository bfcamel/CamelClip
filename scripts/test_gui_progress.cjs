// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
// Pure rendering checks: no browser, network or user data.
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const element={dataset:{},addEventListener(){}};
const sandbox={document:{documentElement:{dataset:{}},querySelector(){return element},addEventListener(){}},window:{addEventListener(){}},location:{hash:'',pathname:'/'},sessionStorage:{getItem(){return ''}},navigator:{language:'en'},setTimeout(){},clearTimeout(){},history:{replaceState(){}},console};
vm.createContext(sandbox);vm.runInContext(fs.readFileSync('internal/ui/gui/web/app.js','utf8'),sandbox);
vm.runInContext("lang='en'",sandbox);
const known=vm.runInContext("metrics({phase_progress:25,progress_estimated:true,part_index:1,part_count:2,downloaded_bytes:2048})",sandbox);
assert.match(known,/25.0%/);assert.match(known,/value="25"/);assert.match(known,/Track 1\/2/);assert.match(known,/2.0 KiB/);assert.match(known,/≈/);
const unknown=vm.runInContext("metrics({downloaded_bytes:4096})",sandbox);assert.match(unknown,/<progress/);assert.doesNotMatch(unknown,/value=/);assert.match(unknown,/4.0 KiB/);assert.match(unknown,/Total size unknown/);
assert.equal(vm.runInContext("sections.some(s=>s[0]==='playlists')",sandbox),true);
console.log('GUI progress and playlist navigation passed.');

const appearanceMessages=[];
sandbox.window.webkit={messageHandlers:{appearance:{postMessage(v){appearanceMessages.push(v)}}}};
vm.runInContext("applyTheme('dark');applyTheme('dark')",sandbox);
assert.equal(sandbox.document.documentElement.dataset.theme,'dark');
assert.equal(appearanceMessages.length,1);
assert.equal(appearanceMessages[0].theme,'dark');
vm.runInContext("applyTheme('light');applyTheme('auto');applyTheme('unsupported')",sandbox);
assert.equal(sandbox.document.documentElement.dataset.theme,'auto');
assert.equal(appearanceMessages.length,3);
vm.runInContext("lang='ru';applyTheme('auto')",sandbox);
assert.equal(appearanceMessages.at(-1).theme,'auto');
console.log('Adaptive theme and native menu synchronization passed.');

const ready=vm.runInContext("jobActions({ID:7,State:'done',OutputPath:'/media/video.mp4',Title:'Video <7>'})",sandbox);
assert.match(ready,/data-action="job-reveal"/);assert.match(ready,/data-id="7"/);assert.match(ready,/Video &lt;7&gt;/);
for(const stage of ['downloading','failed','canceled']){
 assert.doesNotMatch(vm.runInContext(`jobActions({ID:8,State:'${stage}',OutputPath:'',Title:'Pending'})`,sandbox),/job-reveal/);
}
console.log('Per-video reveal actions and unavailable output handling passed.');

vm.runInContext(fs.readFileSync('internal/ui/gui/web/clips.js','utf8'),sandbox);
assert.equal(vm.runInContext("sections.some(s=>s[0]==='clips')",sandbox),true);
assert.equal(vm.runInContext("clipInterval({start_ms:1250,end_ms:4550})",sandbox),'00:00:01.250..00:00:04.550');
vm.runInContext(`clipPage={items:[{id:'batch',status:'completed',created:'2026-10-23T00:00:00Z',source_state:'done',plan:{source:{title:'<script>unsafe</script>'}},tasks:[{id:'task',state:'done',segment:{title:'<clip>',range:{start_ms:0,end_ms:1000}},asset_id:'real-asset'}]}],total:1};lang='en';renderClips()`,sandbox);
assert.match(element.innerHTML,/&lt;script&gt;unsafe&lt;\/script&gt;/);
assert.match(element.innerHTML,/data-id="real-asset"/);
assert.doesNotMatch(element.innerHTML,/<script>/);
console.log('Clip navigation, precise time display, escaping and per-asset actions passed.');

vm.runInContext("state={settings:{cookies_browser:''},browsers:{suggested:'chrome',options:[{value:'chrome',label:'Chrome'}]}}",sandbox);
assert.match(vm.runInContext('settingsFields(true)',sandbox),/value="chrome" selected/);
assert.doesNotMatch(vm.runInContext('settingsFields(false)',sandbox),/value="chrome" selected/);
assert.match(vm.runInContext("browserSelect('firefox:previous')",sandbox),/value="firefox:previous" selected/);

const resourcePanel={innerHTML:''};
let resourceDialogOpen=true;
sandbox.document.querySelector=selector=>selector==='#modal[open] #resource-metrics' && resourceDialogOpen?resourcePanel:null;
vm.runInContext('refreshResourceDialog({CPU:12,Cores:[11,13],RAMUsed:2048})',sandbox);
assert.match(resourcePanel.innerHTML,/12\.0%/);
vm.runInContext('refreshResourceDialog({CPU:34,Cores:[32,36],RAMUsed:4096})',sandbox);
assert.match(resourcePanel.innerHTML,/34\.0%/);
assert.match(resourcePanel.innerHTML,/36\.0%/);
assert.match(resourcePanel.innerHTML,/4\.0 KiB/);
resourceDialogOpen=false;
vm.runInContext('refreshResourceDialog({CPU:78})',sandbox);
assert.doesNotMatch(resourcePanel.innerHTML,/78\.0%/);
