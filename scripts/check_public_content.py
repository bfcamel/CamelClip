#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Check English-only public/installer content and local Markdown links.
Application localization/tests and third-party legal notices are intentionally excluded.
"""
from pathlib import Path
import re
import sys
from urllib.parse import unquote, urlsplit

ROOT=Path(__file__).resolve().parent.parent
markdown=list(ROOT.glob('*.md'))+list((ROOT/'docs').rglob('*.md'))+list((ROOT/'.github').rglob('*.md'))
installer=[ROOT/p for p in ['Start.command','install.sh','build.sh','cmd/camelclip-setup/main.go','scripts/package_setup.py','scripts/build_installers.sh','scripts/build_gui.sh']]
public=markdown+installer+list((ROOT/'.github').rglob('*.yml'))+list((ROOT/'.github').rglob('*.yaml'))
errors=[]
for path in public:
    text=path.read_text()
    for line_number,line in enumerate(text.splitlines(),1):
        if re.search(r'[\u0400-\u04ff]',line):
            errors.append(f'{path.relative_to(ROOT)}:{line_number}: public and installer text must be English; keep application localization in application files')
for path in markdown:
    # Inline images/links only; fenced code samples and external URLs are ignored.
    text=re.sub(r'```.*?```','',path.read_text(),flags=re.S)
    for raw in re.findall(r'!?\[[^\]]*\]\(([^)\s]+)\)',text):
        url=raw.strip('<>');parsed=urlsplit(url)
        if parsed.scheme or url.startswith(('#','/')):continue
        target=unquote(parsed.path)
        if target and not (path.parent/target).exists():
            errors.append(f'{path.relative_to(ROOT)}: broken local link: {url}')
if errors:
    print('\n'.join(errors),file=sys.stderr)
    raise SystemExit(1)
print(f'English-only public content and local links passed ({len(public)} files).')
