#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Isolated GUI playlist lifecycle and live progress using a fixture extractor.
Uses real FFmpeg verification and SQLite; no YouTube account or remote downloads.
"""
import json, os, select, subprocess, sys, tempfile, time, urllib.request, urllib.parse
from pathlib import Path
binary=str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix='camelclip-playlists-') as tmp:
 root=Path(tmp);tools=root/'tools';tools.mkdir();entries=root/'entries.json';entries.write_text(json.dumps(['AAAAAAAAAAA']))
 fixture=root/'fixture.mp4'
 subprocess.run(['ffmpeg','-v','error','-f','lavfi','-i','testsrc2=size=64x64:rate=10:duration=1','-f','lavfi','-i','sine=frequency=440:duration=1','-c:v','libx264','-c:a','aac','-shortest',str(fixture)],check=True)
 extractor=tools/'yt-dlp'
 extractor.write_text('#!'+sys.executable+'\n'+r'''
import json,os,pathlib,shutil,sys,time,urllib.parse
args=sys.argv[1:]
if '--flat-playlist' in args:
 for item in json.loads(pathlib.Path(os.environ['CAMELCLIP_FIXTURE_ENTRIES']).read_text()):
  print(json.dumps({'id':item,'title':'Video '+item}),flush=True)
elif '--dump-single-json' in args:
 print(json.dumps({'id':'fixture','title':'Playlist fixture','duration':1,'formats':[{'height':64,'vcodec':'h264','acodec':'aac'}]}))
elif '--print-to-file' in args:
 out=pathlib.Path(args[args.index('--paths')+1]);manifest=pathlib.Path(args[args.index('--print-to-file')+2]);video=urllib.parse.parse_qs(urllib.parse.urlparse(args[-1]).query)['v'][0];target=out/('video-'+video+'.mp4')
 shutil.copyfile(os.environ['CAMELCLIP_FIXTURE_MEDIA'],target)
 for n in [64,128,192,256]:
  print('CAMELCLIP_PROGRESS '+json.dumps({'downloaded_bytes':n,'total_bytes':256,'speed':128,'eta':2,'filename':str(target)}),flush=True);time.sleep(.35)
 manifest.write_text(json.dumps(str(target))+'\n')
else:
 print('fixture extractor')
''');extractor.chmod(0o755)
 env=dict(os.environ,CAMELCLIP_CONFIG_DIR=str(root/'config'),CAMELCLIP_FIXTURE_ENTRIES=str(entries),CAMELCLIP_FIXTURE_MEDIA=str(fixture),PATH=str(tools)+os.pathsep+os.environ['PATH'])
 process=subprocess.Popen([binary,'--no-open'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
 try:
  assert select.select([process.stdout],[],[],20)[0],'GUI did not start'
  base,token=urllib.parse.urldefrag(process.stdout.readline().strip())
  def request(path,data=None):
   req=urllib.request.Request(base.rstrip('/')+'/api/'+path,headers={'X-CamelClip-Token':token,'Content-Type':'application/json','Origin':base.rstrip('/')},data=None if data is None else json.dumps(data).encode())
   with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)
  def action(name,**values):return request('action',dict(action=name,**values))
  settings=request('state')['settings'];settings['output_directory']=str(root/'media');settings['language']='en';request('settings',settings)
  sub=action('playlist-save',subscription=dict(playlist_id='PLfixture00001',name='Fixture playlist',enabled=True,initial_mode='new_only',interval_minutes=5,request=dict(URL='',Mode='video',Quality='720',Audio='mp3',Out=str(root/'media'),Codec='h264',Container='mp4'),s3_profiles=[]))
  deadline=time.monotonic()+20
  while True:
   page=request('playlists');assert page['total']==1,page
   if page['items'][0]['initialized']:break
   assert time.monotonic()<deadline,page;time.sleep(.1)
  assert request('state')['jobs']==[],'baseline downloaded'
  assert request('playlist-items?id='+sub['id'])['total']==1
  entries.write_text(json.dumps(['AAAAAAAAAAA','BBBBBBBBBBB','AAAAAAAAAAA']));action('playlist-sync',id=sub['id'])
  saw_progress=False;deadline=time.monotonic()+35
  while True:
   snapshot=request('state');jobs=snapshot['jobs']
   for job in jobs:
    progress=job['Monitoring']['Download'].get('phase_progress')
    if progress is not None and 0<progress<100:saw_progress=True
    assert job['State']!='failed',job
   if jobs and jobs[0]['State']=='done':break
   assert time.monotonic()<deadline,snapshot;time.sleep(.1)
  assert saw_progress,'live percentage never appeared'
  assert len(jobs)==1,jobs
  items=request('playlist-items?id='+sub['id']);assert items['total']==2,items
  assert any(v['local_state']=='done' and v['video_id']=='BBBBBBBBBBB' for v in items['items']),items
  action('playlist-pause',id=sub['id']);assert request('playlists')['items'][0]['status']=='paused'
  action('playlist-resume',id=sub['id']);action('playlist-sync',id=sub['id']);time.sleep(1.5)
  assert len(request('state')['jobs'])==1,'reordered entries downloaded again'
  print('GUI playlist baseline, discovery, deduplication, pause/resume and live numerical progress passed.')
 finally:
  process.terminate()
  try:process.wait(timeout=15)
  except subprocess.TimeoutExpired:process.kill();process.wait();raise
  if process.returncode not in (0,-15):raise AssertionError(process.stderr.read())
