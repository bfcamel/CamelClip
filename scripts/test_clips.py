#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Offline clip integration: actual media, durable batches, replay and provenance."""
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile

binary=str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix='camelclip-clips-') as directory:
    root=Path(directory)
    env=dict(os.environ,HOME=str(root),CAMELCLIP_CONFIG_DIR=str(root/'config'),LANG='en_US.UTF-8')
    def run(*args, success=True):
        result=subprocess.run([binary,*args],env=env,capture_output=True,text=True,timeout=60)
        if success: assert result.returncode==0,(args,result.stdout,result.stderr)
        return result
    def data(*args): return json.loads(run(*args).stdout)
    source=root/'Source with spaces.mp4'
    subprocess.run(['ffmpeg','-v','error','-f','lavfi','-i','testsrc2=size=160x90:rate=10:duration=8','-f','lavfi','-i','sine=duration=8','-c:v','libx264','-g','40','-c:a','aac','-shortest',str(source)],check=True)
    original=hashlib.sha256(source.read_bytes()).hexdigest()
    asset=data('library','import',str(source))
    plan=data('clips','plan','--asset',asset['id'],'--range','1.250..4.550 | First','--range','4..6 | Second','--out',str(root/'output'))
    args=('clips','create','--plan-id',plan['id'],'--revision',str(plan['revision']),'--digest',plan['digest'],'--idempotency-key','offline-test')
    batch=data(*args)
    assert data(*args)['id']==batch['id'],'admission replay created a batch'
    assert run('library','delete-file',asset['id'],'--yes',success=False).returncode!=0,'active clip source deleted'
    finished=data('clips','run',batch['id'])
    assert finished['status']=='completed',finished
    assert len(finished['tasks'])==2
    paths=[]
    for task in finished['tasks']:
        path=Path(task['path']); paths.append(path)
        assert path.exists()
        assert hashlib.sha256(path.read_bytes()).hexdigest()==task['evidence']['sha256']
        provenance=data('clips','provenance',task['id'])
        assert len(provenance)==1 and provenance[0]['segment']['range']==task['segment']['range']
        assert not ('source_path' in json.dumps(provenance) or 'browser' in json.dumps(provenance))
    before=[p.stat().st_mtime_ns for p in paths]
    assert data('clips','run',batch['id'])['status']=='completed'
    assert [p.stat().st_mtime_ns for p in paths]==before,'completed clips were rendered again'
    assert hashlib.sha256(source.read_bytes()).hexdigest()==original,'original media changed'
    assert data('library','list','--json')['total']==3
    assert data('clips','list')['total']==1
    assert data('clips','deliveries')==[],'local-only batch admitted an upload'
    other=data('clips','plan','--asset',asset['id'],'--range','0..2','--out',str(root/'output'))
    conflict=run('clips','create','--plan-id',other['id'],'--digest',other['digest'],'--idempotency-key','offline-test',success=False)
    assert conflict.returncode!=0 and data('clips','list')['total']==1
    canceled=data('clips','create','--plan-id',other['id'],'--digest',other['digest'],'--idempotency-key','cancel-test')
    canceled=data('clips','cancel',canceled['id'],'--revision',str(canceled['control_revision']))
    assert canceled['status']=='canceled'
    db=sqlite3.connect(root/'config/queue.sqlite3')
    assert db.execute('pragma user_version').fetchone()[0]==7
    assert db.execute('select count(*) from clip_provenance').fetchone()[0]==2
    assert db.execute('select count(*) from clip_deliveries').fetchone()[0]==0
    db.close()
print('Offline clips: precise ranges, single preparation, durable replay, catalog, provenance, cancellation and local-only delivery passed.')
