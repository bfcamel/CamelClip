#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Refresh distribution notices from the versions pinned in go.mod."""
from pathlib import Path
import re
import subprocess
root = Path(__file__).resolve().parent.parent
cache = Path(subprocess.check_output(['go', 'env', 'GOMODCACHE'], text=True).strip())
notices = ['Third-party license notices for Clipnaro\n\nClipnaro project source is licensed under Apache-2.0. Dependencies retain their own licenses.\nyt-dlp and FFmpeg are external programs and are not bundled.\n']
for module, version in re.findall(r'^\s*([^\s()]+)\s+(v[^\s]+)', (root / 'go.mod').read_text(), re.M):
    directory = cache / (module + '@' + version)
    files = sorted(p for p in directory.iterdir() if p.is_file() and (p.name.upper().startswith(('LICENSE', 'COPYING', 'NOTICE'))))
    if not files:
        raise SystemExit('Missing license for ' + module)
    notices.append('\n' + '=' * 72 + '\n' + module + ' ' + version + '\n' + '=' * 72 + '\n')
    for file in files:
        notices.extend([file.name + '\n', file.read_text(errors='replace'), '\n'])
(root / 'THIRD_PARTY_LICENSES.txt').write_text(''.join(notices).rstrip() + '\n')
