#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Validate setup artifacts; exercise native user installs in temporary prefixes.
No system package is installed, no personal application/config directory is used.
"""
import hashlib
import itertools
import json
import os
from pathlib import Path
import platform
import pty
import select
import time
import plistlib
import shutil
import subprocess
import tarfile
import tempfile
import xml.etree.ElementTree as ET

ROOT=Path(__file__).resolve().parent.parent
from package_setup import OUT
host_os='darwin' if platform.system()=='Darwin' else 'linux'
host_arch='arm64' if platform.machine() in ('arm64','aarch64') else 'amd64'

def run(args,**kwargs):
    return subprocess.run([str(a) for a in args],check=True,capture_output=True,text=True,**kwargs)

def records(prefix):
    return {p.stem:json.loads(p.read_text()) for p in (prefix/'share/CamelClip/installation/components').glob('*.json')}

artifacts=[]
for line in (OUT/'SHA256SUMS').read_text().splitlines():
    sha,name=line.split()
    path=OUT/name
    assert path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest()==sha,name
    artifacts.append(path)
archives=[p for p in artifacts if p.name.endswith('.tar.gz')]
assert len(archives)==4,'build all four platform/architecture archives first'
for archive in archives:
    with tempfile.TemporaryDirectory(prefix='camelclip-installer-test-') as tmp:
        tmp=Path(tmp).resolve()
        with tarfile.open(archive) as tar:
            for member in tar.getmembers():
                path=Path(member.name)
                assert not path.is_absolute() and '..' not in path.parts and member.isfile(), member.name
            tar.extractall(tmp)
        source=next(p.parent for p in tmp.glob('*/manifest.json'))
        manifest=json.loads((source/'manifest.json').read_text())
        assert not any('\u0400' <= c <= '\u04ff' for c in (source/'README.txt').read_text())
        assert manifest['schema']==1
        components={c['id']:c for c in manifest['components']}
        assert set(components)=={'manager','terminal','gui','tui','cli'}
        destinations=set()
        for c in components.values():
            for entry in c['files']:
                key=(entry['area'],entry['path'])
                assert key not in destinations,key
                destinations.add(key)
                data=(source/entry['source']).read_bytes()
                assert len(data)==entry['size'] and hashlib.sha256(data).hexdigest()==entry['sha256']
                assert entry['mode'] in (0o644,0o755)
        setup=(source/'camelclip-setup').read_bytes()
        if manifest['os']=='linux':
            assert setup[:4]==b'\x7fELF'
            machine=int.from_bytes(setup[18:20],'little')
            assert machine==(183 if manifest['arch']=='arm64' else 62)
        else:
            assert setup[:4]==b'\xcf\xfa\xed\xfe'
            cpu=int.from_bytes(setup[4:8],'little')
            assert cpu==(0x100000c if manifest['arch']=='arm64' else 0x1000007)
        if (manifest['os'],manifest['arch'])!=(host_os,host_arch):
            continue
        config=tmp/'config';config.mkdir();(config/'settings.json').write_text('untouched test preferences')
        env=dict(os.environ,CAMELCLIP_CONFIG_DIR=str(config))
        for n in range(1,4):
            for choices in itertools.combinations(['gui','tui','cli'],n):
                prefix=tmp/('install with space and quote\' '+'-'.join(choices))
                command=[source/'setup.sh','--prefix',prefix,'--components',','.join(choices),'--yes']
                run(command,env=env)
                run(command,env=env) # repeatable same-version install
                state=records(prefix)
                expected=set(choices)|{'manager'}
                if set(choices)&{'tui','cli'}: expected.add('terminal')
                assert set(state)==expected,(choices,state.keys())
                manager=prefix/'bin/camelclip-setup'
                assert run([manager,'--status'],env=env).returncode==0 # remembers custom prefix
                if 'cli' in choices:
                    assert 'camelclip' in run([prefix/'bin/camelclip','version'],env=env).stdout.lower()
                if 'gui' in choices and host_os=='darwin':
                    app=prefix/'Applications/CamelClip.app'
                    info=plistlib.loads((app/'Contents/Info.plist').read_bytes())
                    assert (app/'Contents/Resources'/info['CFBundleIconFile']).is_file()
                # Adding CLI retains GUI/TUI choices; removing it retains shared runtime when needed.
                run([source/'setup.sh','--prefix',prefix,'--components','cli','--yes'],env=env)
                assert set(records(prefix))>=expected
                run([manager,'--remove','cli','--yes'],env=env)
                remaining=set(records(prefix))
                assert ('terminal' in remaining)==('tui' in choices)
                if 'cli' not in choices or len(choices)>1:
                    run([manager,'--uninstall','--yes'],env=env)
                assert not records(prefix)
                assert (config/'settings.json').read_text()=='untouched test preferences'
        # Exercise the English-only menu through a terminal, not piped input.
        prefix=tmp/'interactive-menu'
        master,slave=pty.openpty()
        try:
            process=subprocess.Popen([str(source/'setup.sh'),'--prefix',str(prefix)],stdin=slave,stdout=slave,stderr=slave,env=env)
            os.close(slave);slave=None
            os.write(master,b'2\ny\n')
            deadline=time.monotonic()+20
            output=b''
            while process.poll() is None:
                assert time.monotonic()<deadline,'interactive setup hung'
                if select.select([master],[],[],.1)[0]:
                    try: output+=os.read(master,65536)
                    except OSError: break
            process.wait(timeout=10)
            assert process.returncode==0,output.decode(errors='replace')
            assert not any('\u0400' <= c <= '\u04ff' for c in output.decode(errors='replace')), 'Installer output must be English-only'
            assert set(records(prefix))=={'manager','terminal','tui','cli'}
            run([prefix/'bin/camelclip-setup','--uninstall','--yes'],env=env)
        finally:
            os.close(master)
            if slave is not None: os.close(slave)
            if process.poll() is None: process.kill();process.wait()
        # Corruption must be rejected before any executable publication.
        prefix=tmp/'tamper-test'
        cli=components['cli']['files'][0]
        payload=source/cli['source'];payload.write_bytes(b'tampered')
        failure=subprocess.run([str(source/'setup.sh'),'--prefix',str(prefix),'--components','cli','--yes'],env=env,capture_output=True,text=True)
        assert failure.returncode!=0 and not (prefix/'bin/camelclip').exists()
        assert not records(prefix)
    print('Validated',archive.name)

packages=[p for p in artifacts if p.suffix=='.pkg']
assert len(packages)==2,'expected macOS arm64 and amd64 packages'
if host_os=='darwin':
    for package in packages:
        with tempfile.TemporaryDirectory(prefix='camelclip-pkg-inspect-') as tmp:
            root=Path(tmp)/'expanded'
            run(['pkgutil','--expand-full',package,root])
            dist=ET.parse(root/'Distribution').getroot()
            texts=[(root/'Distribution').read_text()]+[p.read_text() for p in (root/'Resources').rglob('*.html')]
            assert all(not any('\u0400' <= c <= '\u04ff' for c in text) for text in texts), 'Package screens must be English-only'
            assert {c.attrib['id'] for c in dist.findall('choice')}=={'manager','terminal','gui','tui','cli'}
            assert dist.find('options').attrib['customize']=='always'
            for name in ['manager','terminal','gui','tui','cli']:
                component=root/(name+'.pkg')
                assert (component/'Scripts/preinstall').is_file()
                assert (component/'Scripts/preflight').is_file()
                incoming=json.loads((component/'Scripts/incoming.json').read_text())
                assert incoming['id']==name
                payload=component/'Payload'
                for entry in incoming['files']:
                    roots={'data':'Library/Application Support/CamelClip','bin':'usr/local/bin','apps':'Applications','desktop':'Applications','icons':'usr/local/share/pixmaps'}
                    data=(payload/roots[entry['area']]/entry['path']).read_bytes()
                    assert hashlib.sha256(data).hexdigest()==entry['sha256']
            print('Inspected',package.name)
print('Setup archives, component ownership, seven native selections, reinstall/add/remove/uninstall, corruption rejection and package contents passed.')
