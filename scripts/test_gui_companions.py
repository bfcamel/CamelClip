#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Authenticated desktop companions, actual local media and offline restart."""
import json,os,select,subprocess,sys,tempfile
from pathlib import Path
from urllib.parse import urldefrag
from urllib.request import Request,urlopen
from urllib.error import HTTPError
binary=str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix='camelclip-gui-companions-') as directory:
 root=Path(directory);env=dict(os.environ,CAMELCLIP_CONFIG_DIR=str(root/'config'));process=None
 def start():
  global process,base,token
  process=subprocess.Popen([binary,'--no-open'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
  assert select.select([process.stdout],[],[],20)[0],'startup timeout'
  base,token=urldefrag(process.stdout.readline().strip());assert base.startswith('http://127.0.0.1:')
 def call(path,data=None,raw=False,authorized=True):
  headers={'Origin':base.rstrip('/'),'Content-Type':'application/json'}
  if authorized:headers['X-CamelClip-Token']=token
  with urlopen(Request(base+'api/'+path,headers=headers,data=None if data is None else json.dumps(data).encode()),timeout=90) as response:return response.read() if raw else json.load(response)
 try:
  start();captions=root/'captions.srt';captions.write_text('1\n00:00:01,000 --> 00:00:03,000\nHello world\n\n2\n00:00:02,000 --> 00:00:04,000\nSecond line\n')
  source=root/'source.mp4';subprocess.run(['ffmpeg','-v','error','-f','lavfi','-i','testsrc2=size=160x90:rate=10:duration=6','-f','lavfi','-i','sine=duration=6','-i',str(captions),'-map','0:v','-map','1:a','-map','2:s','-c:v','libx264','-c:s','mov_text',str(source)],check=True)
  asset=call('action',{'action':'import','path':str(source)});catalog=call('companion-discover',{'source':{'asset_id':asset['id']}})
  poster=call('companion-poster',{'id':catalog['id']});jpeg=call('companion-blob?id='+poster['id'],raw=True);assert jpeg.startswith(b'\xff\xd8')
  try:call('companion-blob?id='+poster['id'],raw=True,authorized=False);raise AssertionError('unauthorized blob exposed')
  except HTTPError as error:assert error.code==401
  snapshot=call('companion-caption',{'id':catalog['id'],'track':catalog['tracks'][0]['id']});page=call('companion-cues?id='+snapshot['id']);assert page['total']==2;assert call('companion-vtt?id='+snapshot['id'],raw=True).startswith(b'WEBVTT')
  interval=call('companion-range',{'id':snapshot['id'],'first':0,'last':1});assert interval=={'start_ms':1000,'end_ms':4000}
  try:call('companion-range',{'id':snapshot['id'],'first':1,'last':0});raise AssertionError('reversed range accepted')
  except HTTPError as error:assert error.code==400
  call('action',{'action':'quit','confirm':True});process.wait(timeout=15);start()
  assert call('companion-cues?id='+snapshot['id'])['total']==2
  assert call('companion-blob?id='+poster['id'],raw=True)==jpeg
  assert len(call('companion-links?owner='+asset['id']))==3
  with urlopen(base+'companions.js') as response:assert b'companionHandle' in response.read()
  call('action',{'action':'quit','confirm':True});process.wait(timeout=15)
 finally:
  if process is not None and process.poll() is None:process.terminate();process.wait(timeout=15)
print('GUI companions passed: auth, poster, caption, range validation and offline restart.')
