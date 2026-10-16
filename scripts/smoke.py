#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Native macOS/Linux end-to-end checks; all data and fake network tools are temporary.
Requires Python 3, FFmpeg and ffprobe. No external network or browser cookies.
"""
from pathlib import Path
import fcntl
import json
import os
import pty
import select
import shutil
import sqlite3
import struct
import subprocess
import sys
import tempfile
import termios
import time

BINARY = Path(sys.argv[1]).resolve()
REPO = Path(__file__).resolve().parent.parent
ACTIVE = []

def wait_for(predicate, timeout=25):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            if predicate():
                return
        except (OSError, sqlite3.DatabaseError):
            pass
        time.sleep(.03)
    raise AssertionError('Timed out waiting for native smoke-test condition')

def jobs(config):
    # Live WAL-aware read; never use immutable=1 while a writer is active.
    with sqlite3.connect(str(config / 'queue.sqlite3'), timeout=1) as db:
        return db.execute('SELECT state,output_path FROM jobs ORDER BY id').fetchall()

with tempfile.TemporaryDirectory(prefix='clipnaro-smoke-') as temporary:
    root = Path(temporary)
    tools = root / 'tools'
    tools.mkdir()
    fixture = root / 'fixture.mp4'
    ffmpeg = shutil.which('ffmpeg')
    assert ffmpeg and shutil.which('ffprobe'), 'FFmpeg and ffprobe are required'
    subprocess.run([ffmpeg, '-v', 'error', '-f', 'lavfi', '-i', 'testsrc2=size=64x64:rate=10:duration=1', '-f', 'lavfi', '-i', 'sine=frequency=440:duration=1', '-c:v', 'libx264', '-c:a', 'aac', '-shortest', str(fixture)], check=True)
    helper = '#!' + sys.executable + '\n' + r'''
from pathlib import Path
import json, os, shutil, sys, time
args = sys.argv[1:]
if '--version' in args:
 print('2026.01.01'); sys.exit(0)
if '--dump-single-json' in args:
 print(json.dumps({'id':'fixture','title':'Fixture','duration':1,'formats':[{'height':64,'vcodec':'h264','acodec':'aac'}]})); sys.exit(0)
marker=Path(os.environ['SMOKE_MARKER'])
if os.environ.get('SMOKE_HOLD_STAGE')=='downloading' and not marker.exists():
 marker.write_text('ready'); time.sleep(1.5)
stage=Path(args[args.index('--paths')+1])
source=stage/'fixture.mp4'
shutil.copyfile(os.environ['SMOKE_FIXTURE'],source)
manifest=Path(args[args.index('--print-to-file')+2])
manifest.write_text(json.dumps(str(source))+'\n')
'''
    (tools / 'yt-dlp').write_text(helper)
    (tools / 'yt-dlp').chmod(0o755)
    wrapper = '#!' + sys.executable + '\n' + r'''
from pathlib import Path
import os, sys, time
args=sys.argv[1:]
stage='verifying' if 'null' in args else 'processing'
marker=Path(os.environ['SMOKE_MARKER'])
if os.environ.get('SMOKE_HOLD_STAGE')==stage and not marker.exists():
 marker.write_text('ready'); time.sleep(1.5)
os.execv(os.environ['SMOKE_FFMPEG'], [os.environ['SMOKE_FFMPEG']]+args)
'''
    (tools / 'ffmpeg').write_text(wrapper)
    (tools / 'ffmpeg').chmod(0o755)
    base_env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ['PATH'], SMOKE_FIXTURE=str(fixture), SMOKE_FFMPEG=ffmpeg, TERM='xterm-256color', LANG='en_US.UTF-8')
    try:
        setup_config = root / 'first-run'
        setup_config.mkdir()
        setup_env = dict(base_env, CLIPNARO_CONFIG_DIR=str(setup_config), SMOKE_HOLD_STAGE='', SMOKE_MARKER=str(root / 'setup-marker'))
        setup_input = '1\n' + str(root / 'first-downloads') + '\n1\n1\n1\n7\n'
        first = subprocess.run([str(BINARY)], input=setup_input, text=True, env=setup_env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=25)
        assert first.returncode == 0, first.stdout
        preferences = json.loads((setup_config / 'settings.json').read_text())
        assert preferences['video_codec'] == 'h264' and preferences.get('cookies_browser', '') == ''
        second = subprocess.run([str(BINARY)], input='7\n', text=True, env=setup_env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=10)
        assert second.returncode == 0 and 'Language / Язык' not in second.stdout, second.stdout
        print('First-run setup, explicit consent and subsequent wizard bypass passed.')
        for stage in ('downloading', 'processing', 'verifying'):
            config = root / stage
            config.mkdir()
            output = config / 'downloads'
            settings = dict(language='en', mode='video', quality='best', audio_format='mp3', output_directory=str(output), video_codec='h264', video_container='mp4')
            (config / 'settings.json').write_text(json.dumps(settings))
            marker = config / 'stage-ready'
            env = dict(base_env, CLIPNARO_CONFIG_DIR=str(config), SMOKE_HOLD_STAGE=stage, SMOKE_MARKER=str(marker))
            command = [str(BINARY), 'download', 'https://example.test/fixture']
            with (config / 'run.log').open('wb') as log:
                first = subprocess.Popen(command, env=env, stdout=log, stderr=log)
                ACTIVE.append(first)
                wait_for(lambda: marker.exists() and jobs(config)[0][0] == stage)
                assert not list(output.glob('*.mp4')), 'Unverified output published'
                first.kill()
                first.wait(timeout=5)
                second = subprocess.Popen(command, env=env, stdout=log, stderr=log)
                ACTIVE.append(second)
                assert second.wait(timeout=25) == 0, (config / 'run.log').read_text()
            rows = jobs(config)
            assert len(rows) == 1 and rows[0][0] == 'done', rows
            assert len(list(output.glob('*.mp4'))) == 1
            subprocess.run([ffmpeg, '-v', 'error', '-xerror', '-i', rows[0][1], '-f', 'null', '-'], check=True)
        print('Abrupt exit/restart during download, processing and verification passed.')
        # Test terminal controls on an actual PTY, including idle polling on Linux.
        env = dict(base_env, CLIPNARO_CONFIG_DIR=str(config), SMOKE_HOLD_STAGE='', SMOKE_MARKER=str(marker))
        master, slave = pty.openpty()
        saved = termios.tcgetattr(slave)
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 130, 0, 0))
        capture = bytearray()
        def drain():
            while select.select([master], [], [], 0)[0]:
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    break
                if not chunk:
                    break
                capture.extend(chunk)
            return bytes(capture)
        panel = subprocess.Popen([str(BINARY)], env=env, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
        ACTIVE.append(panel)
        try:
            wait_for(lambda: b's: Settings' in drain())
            os.write(master, b's')
            wait_for(lambda: b'Video codec' in drain())
            os.write(master, b'\x1b[B' * 3 + b'\r')
            time.sleep(.2)
            drain()
            capture.clear()
            os.write(master, b'\x1b')
            wait_for(lambda: b'Video codec' in drain())
            os.write(master, b'\x1b')
            time.sleep(.2)
            # Status tabs, search and help must remain usable across resizes.
            os.write(master, b'\t\x1b[Z/search text\r')
            time.sleep(.25)
            os.write(master, b'\x1b')
            for rows, columns in ((25, 100), (18, 60), (6, 20), (40, 130)):
                fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', rows, columns, 0, 0))
                time.sleep(1.1)
                drain()
                assert panel.poll() is None, 'Dashboard exited during resize'
            capture.clear()
            os.write(master, b'?')
            wait_for(lambda: b'next / previous tab' in drain())
            os.write(master, b'\x1b')
            time.sleep(.2)
            capture.clear()
            os.write(master, b'2c')
            wait_for(lambda: b'CPU 0' in drain())
            os.write(master, b'\x1b')
            time.sleep(.2)
            os.write(master, b'4u')
            time.sleep(.2)
            capture.clear()
            os.write(master, b'not-a-real-secret')
            time.sleep(.2)
            assert b'not-a-real-secret' not in drain(), 'Secret input was exposed'
            os.write(master, b'\x1b')
            time.sleep(.2)
            os.write(master, b'1q')
            wait_for(lambda: (drain(), panel.poll())[1] is not None)
            assert panel.returncode == 0, capture.decode(errors='replace')
            restored = termios.tcgetattr(slave)
            # BSD may set the transient PENDIN bit when stty reapplies canonical mode.
            restored[3] &= ~getattr(termios, 'PENDIN', 0)
            saved[3] &= ~getattr(termios, 'PENDIN', 0)
            assert restored == saved, ('Terminal state was not restored', saved, restored)
        finally:
            os.close(master)
            os.close(slave)
        print('PTY settings/back navigation, idle refresh and terminal restoration passed.')
        package = root / 'package'
        package.mkdir()
        shutil.copy2(BINARY, package / 'clipnaro')
        shutil.copy2(REPO / 'install.sh', package / 'install.sh')
        home = root / 'home'
        home.mkdir()
        env = dict(os.environ, HOME=str(home), SHELL='/bin/bash')
        install = ['sh', str(package / 'install.sh'), '--add-to-path']
        subprocess.run(install, env=env, check=True, stdout=subprocess.DEVNULL)
        subprocess.run(install + ['--force'], env=env, check=True, stdout=subprocess.DEVNULL)
        assert subprocess.run(install, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0
        profile = home / ('.zprofile' if sys.platform == 'darwin' else '.bashrc')
        assert profile.read_text().count('# Clipnaro CLI') == 1
        assert subprocess.check_output([str(home / '.local/bin/clipnaro'), 'version'], env=env) == subprocess.check_output([str(BINARY), 'version'], env=env)
        print('Packaged installation, upgrade, overwrite protection and PATH idempotence passed.')
    finally:
        for process in ACTIVE:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
