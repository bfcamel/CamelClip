#!/usr/bin/env python3
# Copyright 2026 bfcamel and contributors.
# SPDX-License-Identifier: Apache-2.0
"""Read-only macOS Installer choice/dependency test; never performs installation."""
import itertools
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile

package=Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory(prefix='camelclip-choice-test-') as tmp:
    changes=Path(tmp)/'choices.plist'
    for count in range(1,4):
        for chosen in itertools.combinations(['gui','tui','cli'],count):
            changes.write_bytes(plistlib.dumps([dict(choiceIdentifier=id,choiceAttribute='selected',attributeSetting=int(id in chosen)) for id in ['gui','tui','cli']]))
            result=subprocess.run(['/usr/sbin/installer','-showChoicesAfterApplyingChangesXML',str(changes),'-pkg',str(package),'-target','/'],check=True,capture_output=True)
            rows=plistlib.loads(result.stdout)
            selected={r['choiceIdentifier'] for r in rows if r['choiceAttribute']=='selected' and r['attributeSetting']}
            expected=set(chosen)|{'manager'}
            if set(chosen)&{'tui','cli'}: expected.add('terminal')
            assert selected==expected,(chosen,selected,expected)
            print('Choice dependencies passed:',','.join(chosen))
print('All seven combinations verified by macOS Installer without installation.')
