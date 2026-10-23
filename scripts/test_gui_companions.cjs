// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const elements=new Map();const base={dataset:{},addEventListener(){},textContent:''};
const sandbox={document:{documentElement:{dataset:{}},querySelector(s){return elements.has(s)?elements.get(s):base},addEventListener(){}},window:{addEventListener(){}},location:{hash:'',pathname:'/'},sessionStorage:{getItem(){return ''}},navigator:{language:'en'},setTimeout(){},clearTimeout(){},history:{replaceState(){}},console,URL:{revokeObjectURL(){}},FormData:class{get(k){return sandbox.form[k]}}};
vm.createContext(sandbox);for(const file of ['app','clips','companions'])vm.runInContext(fs.readFileSync(`internal/ui/gui/web/${file}.js`,'utf8'),sandbox);
(async()=>{
 const html=vm.runInContext(`lang='en';companionPanel({id:'catalog',source:{title:'<unsafe>'},posters:true,tracks:[{id:'t',language:'en',origin:'manual',translated:'false'}]})`,sandbox);assert.match(html,/&lt;unsafe&gt;/);assert.doesNotMatch(html,/<unsafe>/);
 elements.set('#dialog-form [name="ranges"]',null);let output={value:''},calls=0;
 sandbox.output=output;sandbox.onAPI=async()=>{calls++;return {start_ms:1250,end_ms:4550}};sandbox.onClip=()=>elements.set('#dialog-form [name="ranges"]',output);
 vm.runInContext(`api=(...a)=>onAPI(...a);toast=()=>{};clipDialog=()=>onClip();companionState={assetID:'asset',first:0,last:1,snapshot:{id:'snapshot'}}`,sandbox);
 await vm.runInContext("companionHandle('companion-range',{})",sandbox);assert.equal(output.value,'00:00:01.250..00:00:04.550');assert.equal(calls,1);
 sandbox.form={source_type:'url',source:'https://example.test/changed',browser:''};vm.runInContext(`companionState={first:0,last:1,snapshot:{id:'s'},request:{source:{url:'https://example.test/original'},browser:''}}`,sandbox);
 await assert.rejects(vm.runInContext("companionHandle('companion-range',{})",sandbox),/Source changed/);assert.equal(calls,1,'stale source reached Core');
 vm.runInContext(`companionState={first:null,last:1,snapshot:{id:'s'}}`,sandbox);await assert.rejects(vm.runInContext("companionHandle('companion-range',{})",sandbox),/Select first cue/);assert.equal(calls,1);
 console.log('GUI companions: escaping, library-to-clips handoff and stale source rejection passed.');
})().catch(e=>{console.error(e);process.exitCode=1});
