#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Isolated GUI clip/S3 integration; uses only generated media and loopback storage."""
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import select
import subprocess
import sys
import tempfile
import threading
import time
from urllib.parse import urldefrag, urlsplit, unquote
from urllib.request import Request, urlopen

binary=str(Path(sys.argv[1]).resolve())
objects={}
class Store(BaseHTTPRequestHandler):
    def log_message(self,*args): pass
    def path_key(self): return unquote(urlsplit(self.path).path)
    def do_HEAD(self):
        path=self.path_key()
        if path in ('/bucket','/bucket/'):
            self.send_response(200);self.end_headers();return
        data=objects.get(path)
        if data is None:self.send_response(404);self.end_headers();return
        self.send_response(200);self.send_header('Content-Length',str(len(data)));self.send_header('ETag','"'+hashlib.md5(data).hexdigest()+'"');self.end_headers()
    def do_PUT(self):
        path=self.path_key();data=self.rfile.read(int(self.headers['Content-Length']))
        if path in objects and self.headers.get('If-None-Match')=='*':self.send_response(412);self.end_headers();return
        objects[path]=data;self.send_response(200);self.send_header('ETag','"'+hashlib.md5(data).hexdigest()+'"');self.end_headers()
    def do_DELETE(self):
        objects.pop(self.path_key(),None);self.send_response(204);self.end_headers()

with tempfile.TemporaryDirectory(prefix='camelclip-gui-clips-') as directory:
    root=Path(directory);env=dict(os.environ,CAMELCLIP_CONFIG_DIR=str(root/'config'))
    store=ThreadingHTTPServer(('127.0.0.1',0),Store);threading.Thread(target=store.serve_forever,daemon=True).start()
    process=None
    def start():
        global process,base,token
        process=subprocess.Popen([binary,'--no-open'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
        assert select.select([process.stdout],[],[],20)[0],'GUI startup timeout'
        base,token=urldefrag(process.stdout.readline().strip());assert base.startswith('http://127.0.0.1:')
    def call(path,data=None):
        request=Request(base.rstrip('/')+'/api/'+path,headers={'Origin':base.rstrip('/'),'X-CamelClip-Token':token,'Content-Type':'application/json'},data=None if data is None else json.dumps(data).encode())
        with urlopen(request,timeout=60) as response:return json.load(response)
    def act(action,**data):return call('action',dict(action=action,**data))
    try:
        start();state=call('state');settings=state['settings'];settings['output_directory']=str(root/'output');settings['language']='en';call('settings',settings)
        with urlopen(base+'clips.js') as response:assert 'renderClips' in response.read().decode()
        source=root/'fixture.mp4';subprocess.run(['ffmpeg','-v','error','-f','lavfi','-i','testsrc2=size=160x90:rate=10:duration=8','-f','lavfi','-i','sine=duration=8','-c:v','libx264','-c:a','aac','-shortest',str(source)],check=True)
        asset=act('import',path=str(source));act('initialize',password='fixture master password')
        for name in ['Selected','Not selected']:
            act('save-profile',profile={'name':name,'region':'us-east-1','bucket':'bucket','endpoint':f'http://127.0.0.1:{store.server_port}','addressing_style':'path_style','enabled':True,'auto_upload':True,'allow_loopback_http':True,'prefix':name.replace(' ','-')},credentials={'accessKey':'fixture-access','secretKey':'fixture-secret','sessionToken':''})
        profiles=call('state')['profiles'];selected=next(p for p in profiles if p['name']=='Selected')
        act('test-write',id=selected['id'],confirm=True)
        request={'source':{'asset_id':asset['id']},'mode':'video','quality':'best','audio':'m4a','accuracy':'precise','output_dir':str(root/'output'),'s3_profiles':[selected['id']]}
        info=call('clip-inspect',request);assert info['duration_ms']>=7900
        plan=call('clip-plan',{'request':request,'ranges':'1.250..4.550 | First\n4..6 | Second'})
        admission={'plan_id':plan['id'],'revision':plan['revision'],'digest':plan['digest'],'idempotency_key':'gui-clips-test'}
        batch=call('clip-admit',admission);assert call('clip-admit',admission)['id']==batch['id']
        deadline=time.monotonic()+45
        while True:
            result=call('clip?id='+batch['id'])
            assert result['status'] not in ('failed','partial_failure'),result
            if result['status']=='completed':break
            assert time.monotonic()<deadline,result;time.sleep(.2)
        while True:
            uploads=call('state')['uploads']
            if len(uploads)==2 and all(u['state']=='done' for u in uploads):break
            assert time.monotonic()<deadline,uploads;time.sleep(.2)
        assert all(u['profile_id']==selected['id'] for u in uploads),'global AutoUpload leaked into clip workflow'
        assert {u['asset_id'] for u in uploads}=={t['asset_id'] for t in result['tasks']}
        assert all('/Selected/' in key for key in objects),objects.keys()
        provenance=call('clip-provenance?id='+result['tasks'][0]['id']);assert len(provenance)==1
        act('quit',confirm=True);process.wait(timeout=15);start()
        restored=call('clip?id='+batch['id']);assert restored['status']=='completed' and len(restored['tasks'])==2
        assert len(call('state')['uploads'])==2,'restart duplicated delivery'
        act('quit',confirm=True);process.wait(timeout=15)
    finally:
        if process is not None and process.poll() is None:process.terminate();process.wait(timeout=15)
        store.shutdown();store.server_close()
print('GUI clips: authenticated routes, actual media, independent S3 selection, provenance and restart replay passed.')
