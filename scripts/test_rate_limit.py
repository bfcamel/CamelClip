#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Measure fixed-share yt-dlp traffic against local HTTP; no external requests.

Requires yt-dlp. Each 10-second measurement follows a five-second warmup.
This checks the conservative allocation, not dynamic redistribution or a hard
instantaneous cap. Metadata requests are outside the download budget.
"""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import json
import re
import shutil
import subprocess
import tempfile
import threading
import time

SIZE = 64 * 1024 * 1024
BUDGET = 2 * 1024 * 1024
MAX_DOWNLOADS = 5
counter = 0
lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        global counter
        match = re.fullmatch(r'bytes=(\d+)-(\d*)', self.headers.get('Range', ''))
        start, end = (int(match[1]), min(int(match[2] or SIZE - 1), SIZE - 1)) if match else (0, SIZE - 1)
        self.send_response(206 if match else 200)
        self.send_header('Content-Type', 'video/mp4')
        self.send_header('Content-Length', str(end - start + 1))
        self.send_header('Accept-Ranges', 'bytes')
        if match:
            self.send_header('Content-Range', f'bytes {start}-{end}/{SIZE}')
        self.end_headers()
        remaining = end - start + 1
        try:
            while remaining:
                size = min(65536, remaining)
                self.wfile.write(b'\0' * size)
                if match:
                    with lock:
                        counter += size
                remaining -= size
        except (BrokenPipeError, ConnectionResetError):
            pass


def measure(server, count):
    executable = shutil.which('yt-dlp')
    assert executable, 'yt-dlp required'
    children = []
    with tempfile.TemporaryDirectory(prefix='camelclip-rate-') as directory:
        try:
            for i in range(count):
                children.append(subprocess.Popen([
                    executable, '--ignore-config', '--no-playlist', '--no-progress',
                    '--fixup', 'never', '--limit-rate', str(BUDGET // MAX_DOWNLOADS),
                    '--http-chunk-size', '256K', '--format', 'best',
                    '--output', str(Path(directory) / f'{i}.mp4'), '--',
                    f'http://127.0.0.1:{server.server_port}/{i}.mp4',
                ], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
            time.sleep(5)
            with lock:
                before = counter
            started = time.monotonic()
            time.sleep(10)
            elapsed = time.monotonic() - started
            with lock:
                transferred = counter - before
            assert all(child.poll() is None for child in children), 'Transfer exited during measurement'
            speed = transferred / elapsed
            assert 0 < speed <= BUDGET * 1.10, (count, speed, BUDGET)
            return {'downloads': count, 'window_seconds': round(elapsed, 3), 'bytes_per_second': round(speed), 'budget_bytes_per_second': BUDGET}
        finally:
            for child in children:
                if child.poll() is None:
                    child.terminate()
            for child in children:
                try:
                    child.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait()


if __name__ == '__main__':
    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        print(json.dumps([measure(server, n) for n in (2, 5)], indent=2))
    finally:
        server.shutdown()
        server.server_close()
