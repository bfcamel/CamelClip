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
assert.equal(appearanceMessages[0].revealTitle,'Reveal');
vm.runInContext("applyTheme('light');applyTheme('auto');applyTheme('unsupported')",sandbox);
assert.equal(sandbox.document.documentElement.dataset.theme,'auto');
assert.equal(appearanceMessages.length,3);
vm.runInContext("lang='ru';applyTheme('auto')",sandbox);
assert.equal(appearanceMessages.at(-1).revealTitle,'Показать в папке');
assert.equal(appearanceMessages.at(-1).fileTitle,'Файл');
console.log('Adaptive theme and native menu synchronization passed.');
