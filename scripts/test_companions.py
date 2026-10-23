#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Offline companions: real embedded captions, durable snapshots, clipping handoff."""
import hashlib,json,os,subprocess,sys,tempfile,sqlite3
from pathlib import Path
binary=str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix='camelclip-companions-') as directory:
 root=Path(directory);env=dict(os.environ,CAMELCLIP_CONFIG_DIR=str(root/'config'),LANG='en_US.UTF-8')
 def run(*args,ok=True):
  p=subprocess.run([binary,*args],env=env,capture_output=True,text=True,timeout=90)
  if ok:assert p.returncode==0,(args,p.stdout,p.stderr)
  return p
 def data(*args):return json.loads(run(*args).stdout)
 captions=root/'captions.srt';captions.write_text('1\n00:00:01,000 --> 00:00:03,000\nHello & goodbye\n\n2\n00:00:02,000 --> 00:00:04,000\nHello & goodbye\n')
 source=root/'source.mp4'
 subprocess.run(['ffmpeg','-v','error','-f','lavfi','-i','testsrc2=size=160x90:rate=10:duration=6','-f','lavfi','-i','sine=duration=6','-i',str(captions),'-map','0:v','-map','1:a','-map','2:s','-c:v','libx264','-c:s','mov_text','-metadata:s:s:0','language=eng',str(source)],check=True)
 original=hashlib.sha256(source.read_bytes()).hexdigest();asset=data('library','import',str(source));catalog=data('companions','inspect','--asset',asset['id']);assert len(catalog['tracks'])==1,catalog
 poster=data('companions','poster','--id',catalog['id']);assert poster['mime']=='image/jpeg' and poster['size']>0
 snap=data('companions','caption','--id',catalog['id'],'--track',catalog['tracks'][0]['id']);assert snap['cue_count']==2,snap
 assert data('companions','caption','--id',catalog['id'],'--track',catalog['tracks'][0]['id'])['id']==snap['id'],'cache replay'
 page=data('companions','cues','--id',snap['id'],'--query','GOODBYE','--limit','1');assert page['total']==2 and page['next_cursor']=='1'
 next_page=data('companions','cues','--id',snap['id'],'--query','GOODBYE','--cursor',page['next_cursor']);assert next_page['items'][0]['index']==1
 interval=data('companions','range','--id',snap['id'],'--first','0','--last','1');assert interval=={'start_ms':1000,'end_ms':4000},interval
 plan=data('clips','plan','--asset',asset['id'],'--range','1..4','--out',str(root/'output'));assert plan['request']['segments'][0]['range']==interval
 exported=root/'output.vtt';data('companions','export','--id',snap['id'],'--file',str(exported));assert exported.read_text().startswith('WEBVTT')
 assert run('companions','export','--id',snap['id'],'--file',str(exported),ok=False).returncode!=0
 imported=data('companions','import','--id',catalog['id'],'--file',str(captions),'--language','en');assert imported['timing']=='unverified'
 assert run('companions','range','--id',imported['id'],'--first','0','--last','1',ok=False).returncode!=0
 links=data('companions','links','--owner',asset['id']);assert len(links)>=3
 stats=data('companions','cache','--prune');assert stats['kept_bytes']>0
 assert hashlib.sha256(source.read_bytes()).hexdigest()==original
 with sqlite3.connect(root/'config'/'queue.sqlite3') as db:assert db.execute('pragma user_version').fetchone()[0]==7
 print('Companion integration passed: poster, captions, replay, search, range, import, export, retention, migration.')
