#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Check checksum coverage, archive contents, executable modes and repeatability."""
import hashlib
from pathlib import Path
import subprocess
import sys
import tarfile
from package import ROOT, source_version

out = ROOT / 'dist'
checksums = (out / 'SHA256SUMS').read_text()
expected = {f'macdown_{source_version()}_{os}_{arch}.tar.gz' for os in ('macos', 'linux') for arch in ('arm64', 'amd64')}
seen = set()
for line in checksums.splitlines():
    digest, name = line.split()
    assert name in expected and name not in seen, name
    seen.add(name)
    assert hashlib.sha256((out / name).read_bytes()).hexdigest() == digest, name
    with tarfile.open(out / name) as archive:
        members = {Path(m.name).name: m for m in archive.getmembers()}
        for required in ('macdown', 'install.sh', 'BUILD.json', 'LICENSE', 'NOTICE', 'THIRD_PARTY_LICENSES.txt', 'README.md', 'CHANGELOG.md'):
            assert required in members, (name, required)
        assert members['macdown'].mode == 0o755
        assert members['install.sh'].mode == 0o755
        data = archive.extractfile(members['macdown']).read(4)
        assert data == b'\x7fELF' if '_linux_' in name else data in (b'\xcf\xfa\xed\xfe', b'\xfe\xed\xfa\xcf')
assert seen == expected
subprocess.run([sys.executable, str(ROOT / 'scripts/package.py')], check=True)
assert (out / 'SHA256SUMS').read_text() == checksums, 'Packaging is not reproducible'
print('Checksums, four architectures, archive contents and repeatability passed.')
