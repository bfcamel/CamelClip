#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Build component installer archives and native macOS packages from prebuilt files."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import subprocess
import tarfile
import tempfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parent.parent
IDENTIFIER = 'com.bfcamel.camelclip'
VERSION = re.search(r'const version = "([^"]+)"', (ROOT/'internal/ui/terminal/cli.go').read_text()).group(1)
OUT = ROOT / 'dist' / 'installers' / VERSION
BUILD = ROOT / 'dist' / 'build'

def digest(data):
    return hashlib.sha256(data).hexdigest()

def component_manifest(directory, system, arch, binaries, app=None, commit='development-local'):
    directory.mkdir(parents=True, exist_ok=True)
    result = dict(schema=1, version=VERSION, commit=commit, os=system, arch=arch, components=[])
    def component(name):
        c = dict(id=name, version=VERSION, files=[])
        result['components'].append(c)
        return c
    def add(c, area, destination, data, mode=0o644, template=False, source=None):
        source = source or ('payload/'+c['id']+'/'+area+'/'+destination)
        path = directory/source
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        path.chmod(mode)
        record = dict(source=source, area=area, path=destination, sha256=digest(data), size=len(data), mode=mode)
        if template:
            record['template'] = True
        c['files'].append(record)
    manager = component('manager')
    add(manager,'data','runtime/camelclip-setup',binaries['setup'].read_bytes(),0o755,source='camelclip-setup')
    add(manager,'bin','camelclip-setup',b'#!/bin/sh\nexec @DATA_SHELL@/runtime/camelclip-setup --prefix @PREFIX_SHELL@ "$@"\n',0o755,True)
    for name in ['LICENSE','NOTICE','THIRD_PARTY_LICENSES.txt']:
        add(manager,'data','notices/'+name,(ROOT/name).read_bytes())
    terminal = component('terminal')
    add(terminal,'data','runtime/camelclip',binaries['terminal'].read_bytes(),0o755)
    gui = component('gui')
    if system == 'darwin':
        if app is None:
            raise ValueError('macOS application bundle required')
        for path in sorted(app.rglob('*')):
            if path.is_symlink():
                raise ValueError('bundle symlinks require explicit packaging support: '+str(path))
            if path.is_file() and path.name != '.DS_Store':
                add(gui,'apps','CamelClip.app/'+path.relative_to(app).as_posix(),path.read_bytes(),0o755 if path.stat().st_mode & 0o111 else 0o644)
    else:
        add(gui,'data','runtime/camelclip-gui',binaries['gui'].read_bytes(),0o755)
        add(gui,'bin','camelclip-gui',f'#!/bin/sh\nexec @DATA_SHELL@/runtime/camelclip-gui "$@"\n'.encode(),0o755,True)
        desktop = '[Desktop Entry]\nType=Application\nName=CamelClip\nComment=Video and audio workspace\nExec="@BIN_DESKTOP@/camelclip-gui"\nIcon=camelclip\nTerminal=false\nCategories=AudioVideo;Network;\n'
        add(gui,'desktop','com.bfcamel.CamelClip.desktop',desktop.encode(),template=True)
        add(gui,'icons','camelclip.png',(ROOT/'internal/ui/gui/web/logo.png').read_bytes())
    tui = component('tui')
    tui_launcher = f'#!/bin/sh\nexec @DATA_SHELL@/runtime/camelclip tui "$@"\n'.encode()
    add(tui,'bin','camelclip-tui',tui_launcher,0o755,True)
    if system == 'darwin':
        add(tui,'apps','CamelClip TUI.command',b'#!/bin/sh\nexport PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"\n'+tui_launcher.split(b'\n',1)[1],0o755,True)
    else:
        add(tui,'desktop','com.bfcamel.CamelClip.TUI.desktop',b'[Desktop Entry]\nType=Application\nName=CamelClip Terminal\nExec="@BIN_DESKTOP@/camelclip-tui"\nTerminal=true\nCategories=AudioVideo;\n',template=True)
    cli = component('cli')
    add(cli,'bin','camelclip',f'#!/bin/sh\nexec @DATA_SHELL@/runtime/camelclip "$@"\n'.encode(),0o755,True)
    # Include Homebrew's standard Linux prefix even for desktop launches.
    if system == 'linux':
        for component in result['components']:
            for entry in component['files']:
                if entry['area'] == 'bin':
                    launcher = directory / entry['source']
                    data = launcher.read_bytes().replace(b'#!/bin/sh\n', b'#!/bin/sh\nexport PATH="$PATH:/home/linuxbrew/.linuxbrew/bin"\n', 1)
                    launcher.write_bytes(data)
                    entry.update(size=len(data), sha256=digest(data))
    manifest = directory/'manifest.json'
    manifest.write_text(json.dumps(result,sort_keys=True,indent=2)+'\n')
    (directory/'setup.sh').write_text('#!/bin/sh\nset -eu\ndir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"\nexec "$dir/camelclip-setup" --source "$dir" "$@"\n')
    (directory/'setup.sh').chmod(0o755)
    (directory/'README.txt').write_text('CamelClip component setup\n\nRun ./setup.sh for the interactive menu.\nOr: ./setup.sh --components gui,tui,cli --yes\nApplication files belong to this user. Dependency installation may require sudo.\nThe Linux GUI opens in your browser.\n\nUpdate: ./setup.sh --components cli --yes (retains and updates other installed choices)\nManage: ~/.local/bin/camelclip-setup --status\nRemove: ~/.local/bin/camelclip-setup --remove gui --yes\nUninstall: ~/.local/bin/camelclip-setup --uninstall --yes\nSettings and downloaded files are retained.\n\nChecksums detect corruption, not publisher identity. Only use a trusted distribution.\nSetup checks yt-dlp, ffmpeg, ffprobe and Deno and provisions missing tools after confirmation.\n')
    return result

def archive(directory, destination, epoch):
    stream = io.BytesIO()
    with tarfile.open(fileobj=stream,mode='w',format=tarfile.PAX_FORMAT) as tar:
        for path in sorted(directory.rglob('*')):
            if not path.is_file():
                continue
            info=tarfile.TarInfo(directory.name+'/'+path.relative_to(directory).as_posix())
            data=path.read_bytes()
            info.size=len(data); info.mode=0o755 if path.stat().st_mode & 0o111 else 0o644
            info.mtime=epoch
            tar.addfile(info,io.BytesIO(data))
    destination.write_bytes(gzip.compress(stream.getvalue(),mtime=0))

def system_destination(area,path):
    roots={'data':'Library/Application Support/CamelClip','bin':'usr/local/bin','apps':'Applications','desktop':'Applications','icons':'usr/local/share/pixmaps'}
    return Path(roots[area])/path

def render(data, component):
    data=data.replace(b'@DATA_SHELL@',b"'/Library/Application Support/CamelClip'").replace(b'@PREFIX_SHELL@',b"'/usr/local'").replace(b'@BIN_DESKTOP@',b'/usr/local/bin')
    if component=='manager' and b'--prefix ' in data:
        data=data.replace(b"--prefix '/usr/local'",b'--scope system')
    return data

def mac_package(source, manifest, arch, destination):
    with tempfile.TemporaryDirectory(prefix='camelclip-pkg-') as tmp:
        tmp=Path(tmp);packages=tmp/'packages';packages.mkdir();resources=tmp/'resources';resources.mkdir()
        prefix='Library/Application Support/CamelClip'
        for c in manifest['components']:
            name=c['id'];root=tmp/('root-'+name);root.mkdir();records=[]
            for entry in c['files']:
                data=(source/entry['source']).read_bytes()
                if entry.get('template'):
                    data=render(data,name)
                path=root/system_destination(entry['area'],entry['path']);path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(data);path.chmod(entry['mode'])
                records.append({**{k:v for k,v in entry.items() if k not in ('source','template')},'sha256':digest(data),'size':len(data)})
            record=root/prefix/'installation'/'components'/(name+'.json');record.parent.mkdir(parents=True,exist_ok=True)
            record.write_text(json.dumps(dict(id=name,version=VERSION,files=records),sort_keys=True,indent=2)+'\n')
            scripts=tmp/('scripts-'+name);scripts.mkdir()
            (scripts/'incoming.json').write_text(json.dumps(dict(id=name,version=VERSION,files=records)))
            shutil.copy2(source/'camelclip-setup',scripts/'preflight')
            (scripts/'preflight').chmod(0o755)
            # Refuse to change executables used by a running local workspace. This
            # deliberately covers all accounts for the system-wide package.
            (scripts/'preinstall').write_text('''#!/bin/sh
set -eu
if [ "$3" != / ]; then echo 'Install to the running system volume only.' >&2; exit 1; fi
for executable in camelclip camelclip-gui CamelClipWindow; do
 if /usr/bin/pgrep -x "$executable" >/dev/null 2>&1; then
  echo "Close all CamelClip GUI/TUI/CLI sessions before installing." >&2; exit 1
 fi
done
scriptdir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
"$scriptdir/preflight" --scope system --preflight-record "$scriptdir/incoming.json"
exit 0
''')
            (scripts/'preinstall').chmod(0o755)
            if name=='manager':
                hook = scripts/'pkg-dependencies.sh'
                shutil.copy2(ROOT/'scripts/pkg-dependencies.sh',hook)
                pre = scripts/'preinstall'
                pre.write_text(pre.read_text().replace('exit 0', '/bin/sh "$scriptdir/pkg-dependencies.sh" "$scriptdir/preflight"'))
            args=['pkgbuild','--root',str(root),'--identifier',IDENTIFIER+'.'+name,'--version',VERSION,'--install-location','/','--scripts',str(scripts)]
            if name=='gui':
                plist=tmp/'bundles.plist'
                plist.write_bytes(plistlib.dumps([dict(RootRelativeBundlePath='Applications/CamelClip.app',BundleIsRelocatable=False,BundleIsVersionChecked=True,BundleHasStrictIdentifier=True,BundleOverwriteAction='upgrade')]))
                args+=['--component-plist',str(plist)]
            args.append(str(packages/(name+'.pkg')))
            subprocess.run(args,check=True)
        (resources/'welcome.html').write_text('''<html><body><h1>CamelClip</h1><p>Choose GUI, TUI and/or CLI.</p><p>GUI is the graphical application. TUI opens a terminal workspace. CLI installs shell commands. Terminal files are shared automatically.</p><p>Existing components left unchecked are kept, not removed. Use camelclip-setup for explicit removal.</p><p>Close CamelClip before installing. User settings and media are retained.</p><p>This development package is unsigned. Continue to install missing yt-dlp, FFmpeg/ffprobe and Deno with Homebrew. If Homebrew is missing, setup installs it first. A Terminal window may request administrator authentication.</p></body></html>''')
        (resources/'conclusion.html').write_text('''<html><body><h1>Installation complete</h1><p>GUI: /Applications/CamelClip.app<br>TUI: /Applications/CamelClip TUI.command or camelclip-tui<br>CLI: camelclip help</p><p>Setup verified yt-dlp, FFmpeg, ffprobe and Deno.</p><p>Manage: camelclip-setup --status<br>Remove: sudo camelclip-setup --remove gui --yes<br>Uninstall: sudo camelclip-setup --uninstall --yes</p><p>Existing ~/.local/bin/camelclip may take precedence in your PATH. Review `command -v camelclip` if you used an older installer.</p></body></html>''')
        (resources/'license.txt').write_bytes((ROOT/'LICENSE').read_bytes())
        dist=ET.Element('installer-gui-script',{'minSpecVersion':'2'})
        ET.SubElement(dist,'title').text='CamelClip '+VERSION
        ET.SubElement(dist,'options',{'customize':'always','require-scripts':'true','hostArchitectures':'arm64' if arch=='arm64' else 'x86_64'})
        ET.SubElement(dist,'domains',{'enable_anywhere':'false','enable_currentUserHome':'false','enable_localSystem':'true'})
        ET.SubElement(dist,'welcome',{'file':'welcome.html','mime-type':'text/html'})
        ET.SubElement(dist,'conclusion',{'file':'conclusion.html','mime-type':'text/html'})
        ET.SubElement(dist,'license',{'file':'license.txt','mime-type':'text/plain'})
        allowed=ET.SubElement(dist,'allowed-os-versions');ET.SubElement(allowed,'os-version',{'min':'12.0'})
        # Initial choices are evaluated once; later user selections stay editable.
        ET.SubElement(dist,'script').text='''function defaultChoice(id) {
 var base = '/Library/Application Support/CamelClip/installation/components/';
 var existing = system.files.fileExistsAtPath(base + id + '.json');
 if (existing) { return true; }
 if (id != 'gui') { return false; }
 return !system.files.fileExistsAtPath(base + 'tui.json') && !system.files.fileExistsAtPath(base + 'cli.json');
}
'''
        outline=ET.SubElement(dist,'choices-outline')
        for id in ['manager','terminal','gui','tui','cli']: ET.SubElement(outline,'line',{'choice':id})
        for id in ['manager','terminal','gui','tui','cli']:
            attrs=dict(id=id,title={'manager':'Shared setup and notices','terminal':'Shared terminal runtime','gui':'GUI — Graphical application','tui':'TUI — Terminal workspace','cli':'CLI — Shell commands'}[id],description={'gui':'Install CamelClip.app in Applications.','tui':'Install terminal workspace launchers.','cli':'Install the camelclip command.','manager':'Shared maintenance utility and license notices.','terminal':'One runtime shared by CLI and TUI.'}[id])
            if id in ['manager','terminal']:
                attrs.update(visible='false',enabled='false',selected='choices.gui.selected || choices.tui.selected || choices.cli.selected' if id=='manager' else 'choices.tui.selected || choices.cli.selected')
            else: attrs['start_selected']=f"defaultChoice('{id}')"
            choice=ET.SubElement(dist,'choice',attrs);ET.SubElement(choice,'pkg-ref',{'id':IDENTIFIER+'.'+id})
            ref=ET.SubElement(dist,'pkg-ref',{'id':IDENTIFIER+'.'+id,'version':VERSION});ref.text=id+'.pkg'
            if id=='gui':
                close=ET.SubElement(ref,'must-close');ET.SubElement(close,'app',{'id':'com.bfcamel.camelclip.gui'})
        xml=tmp/'Distribution.xml';ET.ElementTree(dist).write(xml,encoding='utf-8',xml_declaration=True)
        args=['productbuild','--distribution',str(xml),'--package-path',str(packages),'--resources',str(resources)]
        if os.environ.get('INSTALLER_SIGN_ID'): args+=['--sign',os.environ['INSTALLER_SIGN_ID']]
        subprocess.run(args+[str(destination)],check=True)
        shutil.copy2(xml,destination.with_suffix('.distribution.xml'))

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--os',choices=['darwin','linux'],required=True)
    parser.add_argument('--arch',choices=['arm64','amd64'],required=True)
    parser.add_argument('--pkg',action='store_true',help='also build native macOS .pkg')
    args=parser.parse_args();OUT.mkdir(parents=True,exist_ok=True)
    system,arch=args.os,args.arch;label='macos' if system=='darwin' else 'linux'
    directory=OUT/f'camelclip_setup_{VERSION}_{label}_{arch}'
    # Never reuse an old payload tree: it may contain removed release files.
    with tempfile.TemporaryDirectory(prefix='camelclip-setup-package-') as tmp:
        stage=Path(tmp)/directory.name
        binaries={key:BUILD/f'{name}-{label}-{arch}' for key,name in [('terminal','camelclip'),('gui','camelclip-gui'),('setup','camelclip-setup')]}
        app=BUILD/f'CamelClip-macos-{arch}.app' if system=='darwin' else None
        manifest=component_manifest(stage,system,arch,binaries,app,os.environ.get('RELEASE_COMMIT','development-local'))
        archive(stage,Path(str(directory)+'.tar.gz'),int(os.environ.get('SOURCE_DATE_EPOCH','0')))
        if args.pkg:
            if system!='darwin': parser.error('--pkg requires --os darwin')
            mac_package(stage,manifest,arch,Path(str(directory)+'.pkg'))
    if system=='linux':
        archive_sha=digest(Path(str(directory)+'.tar.gz').read_bytes())
        target=OUT/f'camelclip_setup_{VERSION}_windows_{arch}.exe'
        subprocess.run(['go','build','-trimpath','-ldflags',f'-s -w -X main.version={VERSION} -X main.digest={archive_sha}','-o',str(target),'./cmd/camelclip-wsl-setup'],cwd=ROOT,env={**os.environ,'GOOS':'windows','GOARCH':arch,'CGO_ENABLED':'0'},check=True)
    artifacts=[p for p in OUT.glob(f'camelclip_setup_{VERSION}_*') if p.suffix in ('.pkg','.exe') or p.name.endswith('.tar.gz')]
    (OUT/'SHA256SUMS').write_text(''.join(digest(p.read_bytes())+'  '+p.name+'\n' for p in sorted(artifacts)))
    print('Packaged',directory.name)

if __name__=='__main__':main()
