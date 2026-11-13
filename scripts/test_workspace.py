#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Loopback-only CLI integration: encrypted profiles, real media index and S3."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit,unquote
import hashlib
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import threading

binary = str(Path(sys.argv[1]).resolve())
objects = {}
lock = threading.Lock()
class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_HEAD(self):
        self.path=unquote(urlsplit(self.path).path)
        with lock:
            if self.path in ('/bucket', '/bucket/'):
                self.send_response(200); self.end_headers(); return
            data = objects.get(self.path)
        if data is None:
            self.send_response(404); self.end_headers(); return
        self.send_response(200)
        self.send_header('Content-Length', str(len(data)))
        self.send_header('ETag', '"' + hashlib.md5(data).hexdigest() + '"')
        self.end_headers()
    def do_PUT(self):
        self.path=unquote(urlsplit(self.path).path)
        data = self.rfile.read(int(self.headers['Content-Length']))
        with lock:
            conflict = self.path in objects and self.headers.get('If-None-Match') == '*'
            if not conflict: objects[self.path] = data
        if conflict:
            data = b'<Error><Code>PreconditionFailed</Code><Message>test-private-error</Message></Error>'
            self.send_response(412); self.send_header('Content-Type', 'application/xml'); self.send_header('Content-Length', str(len(data))); self.end_headers(); self.wfile.write(data); return
        self.send_response(200); self.send_header('ETag', '"' + hashlib.md5(data).hexdigest() + '"'); self.end_headers()
    def do_DELETE(self):
        self.path=unquote(urlsplit(self.path).path)
        with lock: objects.pop(self.path, None)
        self.send_response(204); self.end_headers()

with tempfile.TemporaryDirectory(prefix='camelclip-workspace-') as directory:
    root = Path(directory)
    env = dict(os.environ, HOME=str(root), CAMELCLIP_CONFIG_DIR=str(root/'config'), LANG='en_US.UTF-8')
    def run(args, secrets=(), success=True):
        descriptors=[]; expanded=list(args)
        try:
            for flag, value in secrets:
                read, write = os.pipe(); os.write(write, value); os.close(write); descriptors.append(read); expanded.extend([flag, str(read)])
            result=subprocess.run([binary]+expanded, env=env, pass_fds=descriptors, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
            if success: assert result.returncode==0, (args, result.returncode, result.stdout.decode(), result.stderr.decode())
            return result
        finally:
            for fd in descriptors: os.close(fd)
    source=root/'fixture.mp4'
    subprocess.run(['ffmpeg','-v','error','-f','lavfi','-i','testsrc2=size=64x64:rate=10:duration=1','-f','lavfi','-i','sine=duration=1','-c:v','libx264','-c:a','aac','-shortest',str(source)],check=True)
    asset=json.loads(run(['library','import',str(source)]).stdout)
    assert asset['kind']=='video' and asset['width']==64
    server=ThreadingHTTPServer(('127.0.0.1',0),Handler)
    threading.Thread(target=server.serve_forever,daemon=True).start()
    password=b'workspace master password'
    credentials=json.dumps({'access_key_id':'private-access','secret_access_key':'private-secret','session_token':'private-token'}).encode()
    try:
        run(['storage','profiles','add','--name','Local','--endpoint',f'http://127.0.0.1:{server.server_port}','--region','us-east-1','--bucket','bucket','--style','path_style','--allow-loopback-http'],[('--password-fd',password),('--credentials-fd',credentials)])
        profiles=json.loads(run(['storage','profiles','list']).stdout)
        assert len(profiles)==1
        profile=profiles[0]['id']
        run(['storage','profiles','test',profile],[('--password-fd',password)])
        assert not objects, 'Read-only test wrote an object'
        run(['storage','profiles','test-write',profile,'--yes'],[('--password-fd',password)])
        assert not objects, 'Write test left its object behind'
        result=run(['storage','upload',asset['id'],'--profile',profile,'--json'],[('--password-fd',password)])
        uploaded=json.loads(result.stdout)
        assert len(uploaded)==1 and uploaded[0]['state']=='done' and uploaded[0]['verification']=='size_verified', uploaded
        assert list(objects.values())==[source.read_bytes()]
        again=json.loads(run(['storage','upload',asset['id'],'--profile',profile,'--json'],[('--password-fd',password)]).stdout)
        assert again[0]['id']==uploaded[0]['id'], 'Duplicate upload job created'
        original=source.read_bytes()
        source.unlink()
        assert json.loads(run(['library','refresh',asset['id']]).stdout)['status']=='missing'
        run(['library','delete-file',asset['id'],'--yes'])
        assert json.loads(run(['library','list','--json']).stdout)['total']==0
        source.write_bytes(original)
        run(['library','scan',str(root)])
        restored=json.loads(run(['library','list','--json']).stdout)
        assert restored['total']==1
        run(['library','delete-file',restored['items'][0]['id'],'--yes'])
        assert not source.exists()
        assert json.loads(run(['library','list','--json']).stdout)['total']==0
        dbpath=root/'config/queue.sqlite3'
        for file in dbpath.parent.glob('queue.sqlite3*'):
            data=file.read_bytes()
            for secret in (password,b'private-access',b'private-secret',b'private-token'):
                assert secret not in data, 'Plaintext secret persisted'
        config=run(['config']).stdout
        assert b'private-secret' not in config and b'127.0.0.1' not in config
        with sqlite3.connect(dbpath) as db:
            assert db.execute('pragma integrity_check').fetchone()[0]=='ok'
        print('Workspace CLI, real media index, local S3, idempotence, missing-file refresh and deletion and encrypted storage passed.')
    finally:
        server.shutdown();server.server_close()
