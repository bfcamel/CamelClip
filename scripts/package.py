#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Package already-built binaries reproducibly; no external Python dependencies."""
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parent.parent

def source_version():
    return re.search(r'const version = "([^"]+)"', (ROOT / 'internal/ui/terminal/cli.go').read_text()).group(1)

def package():
    version = source_version()
    tag = os.environ.get('RELEASE_TAG', 'v' + version)
    if tag != 'v' + version:
        raise SystemExit('Release tag must match the application version: v' + version)
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    epoch = int(os.environ.get('SOURCE_DATE_EPOCH') or subprocess.check_output(['git', 'show', '-s', '--format=%ct', 'HEAD'], cwd=ROOT, text=True))
    out = ROOT / 'dist'
    archives = []
    for system in ('macos', 'linux'):
        for arch in ('arm64', 'amd64'):
            name = f'macdown_{version}_{system}_{arch}'
            binary = out / f'macdown-{system}-{arch}'
            files = {n: ((ROOT / n).read_bytes(), 0o755 if n.endswith('.sh') else 0o644) for n in ('README.md', 'LICENSE', 'NOTICE', 'THIRD_PARTY_LICENSES.txt', 'CHANGELOG.md', 'install.sh')}
            files['macdown'] = (binary.read_bytes(), 0o755)
            if system == 'macos':
                files['Start.command'] = ((ROOT / 'Start.command').read_bytes(), 0o755)
            files['BUILD.json'] = (json.dumps({'version': version, 'commit': commit, 'os': system, 'arch': arch}, sort_keys=True, indent=2).encode() + b'\n', 0o644)
            stream = io.BytesIO()
            with tarfile.open(fileobj=stream, mode='w', format=tarfile.USTAR_FORMAT) as tar:
                for filename, (data, mode) in sorted(files.items()):
                    info = tarfile.TarInfo(name + '/' + filename)
                    info.size, info.mode, info.mtime = len(data), mode, epoch
                    tar.addfile(info, io.BytesIO(data))
            destination = out / (name + '.tar.gz')
            destination.write_bytes(gzip.compress(stream.getvalue(), mtime=0))
            archives.append(destination)
    (out / 'SHA256SUMS').write_text(''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in archives))
    print(f'Packaged {len(archives)} archives with SHA256SUMS for {version}.')

if __name__ == '__main__':
    package()
