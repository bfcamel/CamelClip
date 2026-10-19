#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Collect pinned module and Go runtime notices, including nested attributions."""
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parent.parent

def legal_files(directory):
    pattern = re.compile(r'^(LICENSE|LICENCE|COPYING|NOTICE|PATENTS|AUTHORS|COPYRIGHT)(?:$|[.\-_])', re.I)
    return sorted(p for p in directory.rglob('*') if p.is_file()
                  and pattern.match(p.name)
                  and not any(part in ('testdata', '.git', '.github') for part in p.relative_to(directory).parts))

def section(title, directory, files):
    result = ['\n' + '=' * 72 + '\n' + title + '\n' + '=' * 72 + '\n']
    for file in files:
        result.extend([str(file.relative_to(directory)) + '\n', file.read_text(errors='replace'), '\n'])
    return ''.join(result)

def generate():
    raw = subprocess.check_output(['go', 'list', '-m', '-json', 'all'], cwd=ROOT, text=True)
    decoder = json.JSONDecoder()
    modules = []
    while raw.strip():
        module, end = decoder.raw_decode(raw.lstrip())
        raw = raw.lstrip()[end:]
        if not module.get('Main'):
            modules.append(module)
    notices = ['Third-party license notices for CamelClip\n\nCamelClip project source is licensed under Apache-2.0. Dependencies retain their own licenses.\nyt-dlp, FFmpeg and Deno are external programs and are not bundled.\n']
    goroot = Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip())
    runtime_files = [p for p in (goroot / 'LICENSE', goroot / 'PATENTS') if p.is_file()]
    runtime_files += legal_files(goroot / 'src' / 'vendor')
    notices.append(section(subprocess.check_output(['go', 'version'], text=True).strip(), goroot, runtime_files))
    for module in sorted(modules, key=lambda m: m['Path']):
        if module.get('Replace'):
            raise SystemExit('Review replacement module licenses explicitly: ' + module['Path'])
        if not module.get('Dir'):
            raise SystemExit('Run go mod download all before generating notices: ' + module['Path'])
        directory = Path(module['Dir'])
        files = legal_files(directory)
        if not any(p.name.upper().startswith(('LICENSE', 'LICENCE', 'COPYING')) for p in files):
            raise SystemExit('Missing license for ' + module['Path'])
        notices.append(section(module['Path'] + ' ' + module['Version'], directory, files))
    (ROOT / 'THIRD_PARTY_LICENSES.txt').write_text(''.join(notices).rstrip() + '\n')
    print(f'Collected notices for {len(modules)} modules and the Go runtime.')

if __name__ == '__main__':
    generate()
