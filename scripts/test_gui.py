#!/usr/bin/env python3
"""Isolated GUI HTTP smoke: real local media, shared lock, persistence, vault.
Never uses the user's config, browser profiles, credentials or remote sites.
"""
import contextlib
import http.server
import json
import os
from pathlib import Path
import select
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

binary = str(Path(sys.argv[1] if len(sys.argv) > 1 else 'dist/CamelClip.app/Contents/MacOS/camelclip-gui').resolve())
with tempfile.TemporaryDirectory(prefix='camelclip-gui-smoke-') as tmp:
    root = Path(tmp)
    env = dict(os.environ, CAMELCLIP_CONFIG_DIR=str(root / 'config'))
    process = subprocess.Popen([binary, '--no-open'], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    def start_url(p):
        if not select.select([p.stdout], [], [], 20)[0]:
            raise AssertionError('GUI did not report a local URL')
        url = p.stdout.readline().strip()
        assert url.startswith('http://127.0.0.1:'), (url, p.stderr.read() if p.poll() is not None else '')
        return urllib.parse.urldefrag(url)
    base, token = start_url(process)
    def request(path, data=None, auth=True):
        headers = {'Origin': base.rstrip('/'), 'Content-Type': 'application/json'}
        if auth:
            headers['X-CamelClip-Token'] = token
        req = urllib.request.Request(base.rstrip('/')+'/api/'+path, headers=headers, data=None if data is None else json.dumps(data).encode())
        with urllib.request.urlopen(req, timeout=60) as r:
            return json.load(r)
    def action(name, **kwargs):
        return request('action', dict(action=name, **kwargs))
    try:
        try:
            request('state', auth=False)
            raise AssertionError('anonymous request accepted')
        except urllib.error.HTTPError as e:
            assert e.code == 401
        initial = request('state')
        assert initial['firstRun']
        settings = initial['settings']
        settings['output_directory'] = str(root/'media')
        settings['language'] = 'en'
        request('settings', settings)
        second = subprocess.run([binary, '--no-open'], env=env, capture_output=True, text=True, timeout=20)
        assert second.returncode != 0 and 'http://' not in second.stdout, second
        subprocess.run(['ffmpeg', '-hide_banner', '-loglevel', 'error', '-f', 'lavfi', '-i', 'testsrc2=size=160x90:rate=10', '-f', 'lavfi', '-i', 'sine=frequency=440', '-t', '1', '-c:v', 'libx264', '-c:a', 'aac', str(root/'fixture.mp4')], check=True)
        class Handler(http.server.SimpleHTTPRequestHandler):
            def __init__(self, *args, **kwargs):
                super().__init__(*args, directory=str(root), **kwargs)
            def log_message(self, *_):
                pass
        media = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        threading.Thread(target=media.serve_forever, daemon=True).start()
        try:
            url = f'http://127.0.0.1:{media.server_port}/fixture.mp4'
            result = action('submit', urls=[url], choose=True)
            assert len(result) == 1 and not result[0].get('error'), result
            job_id = str(result[0]['id'])
            deadline = time.monotonic()+30
            while True:
                jobs = request('state')['jobs']
                if jobs[0]['State'] == 'choose':
                    break
                assert jobs[0]['State'] != 'failed', jobs
                assert time.monotonic() < deadline, jobs
                time.sleep(.2)
            action('choose', id=job_id, value='best')
            deadline = time.monotonic()+90
            while True:
                jobs = request('state')['jobs']
                if jobs[0]['State'] == 'done':
                    break
                assert jobs[0]['State'] != 'failed', jobs
                assert time.monotonic() < deadline, jobs
                time.sleep(.3)
            assert Path(jobs[0]['OutputPath']).is_file()
            lib = request('library')
            assert lib['total'] == 1, lib
            asset = lib['items'][0]
            assert asset['local_verification'], asset
            action('remove', id=job_id, confirm=True)
            assert not request('state')['jobs'] and request('library')['total'] == 1
            assert Path(asset['path']).is_file()
            # Exercise vault lifecycle using disposable test credentials only.
            password = 'GUI-smoke-master-2026!'
            action('initialize', password=password)
            action('save-profile', profile={'name':'Fixture only','region':'us-east-1','bucket':'gui-smoke','addressing_style':'auto','enabled':True,'key_template':'YYYY/MM/{filename}'}, credentials={'accessKey':'GUI_TEST_ACCESS_KEY','secretKey':'GUI_TEST_SECRET_KEY','sessionToken':''})
            state = request('state')
            profile_id = state['profiles'][0]['id']
            preview = action('preview-upload', assetIds=[asset['id']])
            assert preview[0]['keys'][profile_id].endswith(Path(asset['path']).name)
            action('rotate', password=password, newPassword=password+'new')
            action('lock')
            assert request('state')['locked']
            action('unlock', password=password+'new')
            assert not request('state')['locked']
            for path in (root/'config').glob('queue.sqlite3*'):
                if path.is_file():
                    data = path.read_bytes()
                    assert b'GUI_TEST_SECRET_KEY' not in data and b'GUI_TEST_ACCESS_KEY' not in data
            state_json = json.dumps(request('state'))
            assert 'GUI_TEST_SECRET_KEY' not in state_json and password not in state_json
            action('lock')
            action('quit', confirm=True)
            process.wait(timeout=20)
            assert process.returncode == 0
            # Reopen with preserved preferences, library, encrypted profile and relocked vault.
            process = subprocess.Popen([binary, '--no-open'], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            base, token = start_url(process)
            restored = request('state')
            assert not restored['firstRun'] and restored['locked'] and restored['hasVault']
            assert restored['profiles'][0]['id'] == profile_id
            assert request('library')['total'] == 1
            Path(asset['path']).unlink()
            for operation in ('refresh', 'delete-file'):
                results = action('library-bulk', assetIds=[asset['id']], value=operation, confirm=True)
                assert results == [{'id': asset['id']}], results
                if operation == 'refresh':
                    assert request('library')['items'][0]['status'] == 'missing'
            assert request('library')['total'] == 0
            action('quit', confirm=True)
            process.wait(timeout=20)
            assert process.returncode == 0
        finally:
            media.shutdown()
    finally:
        if process.poll() is None:
            process.terminate()
            process.wait(timeout=20)
    print('GUI smoke passed: media download/verification, quality choice, shared lock, retained library, encrypted vault and restart.')
