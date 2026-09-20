#!/usr/bin/env python3
"""Behavior tests for CHROTE's store-scoped Beads launcher."""
from __future__ import annotations

import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = Path(os.environ.get('CHROTE_BD_WRAPPER_UNDER_TEST', ROOT / 'bin' / 'bd'))
REAL_BD = ROOT / 'vendor' / '@beads' / 'bd' / 'bin' / 'bd.js'
FAKE_BD = textwrap.dedent('''\
    const fs = require('fs');
    const path = require('path');
    const args = process.argv.slice(2);
    if (args[0] === 'help') {
      process.stdout.write(`Flags:
      --title string             Title
      --notes string             Notes
      --json                     JSON
      --hook-json                Hook JSON
      --global                   Global store
  -q, --quiet                    Quiet
  -C, --directory string         Directory
      --db string                Database
`);
      process.exit(0);
    }
    if (args.slice(-2).join(' ') === 'where --json') {
      process.stderr.write('private resolver diagnostic\\n');
      let cwd = process.cwd(), database;
      for (let i = 0; i < args.length - 2; i++) {
        if (args[i] === '-C' || args[i] === '--directory') cwd = args[++i];
        else if (args[i] === '--db') database = args[++i];
      }
      const store = database ? path.dirname(path.resolve(cwd, database)) :
        path.resolve(cwd, process.env.BEADS_DIR || '.beads');
      process.stdout.write(process.env.BD_TEST_WHERE_OUTPUT ?? JSON.stringify({path: store}));
      process.exit(Number(process.env.BD_TEST_WHERE_STATUS || 0));
    }
    fs.writeFileSync(process.env.BD_TEST_ARGS, JSON.stringify(args));
    if (process.env.BD_TEST_MANIFEST) {
      const file = process.env.BD_TEST_MANIFEST;
      if ((fs.statSync(file).mode & 0o777) !== 0o660) process.exit(92);
      fs.writeFileSync(file + '.new', 'new manifest\\n', {mode: 0o600});
      fs.chmodSync(file + '.new', 0o600);
      fs.renameSync(file + '.new', file);
    }
    process.stdout.write(process.env.BD_TEST_STDOUT || '');
    process.stderr.write(process.env.BD_TEST_STDERR || '');
    process.exit(Number(process.env.BD_TEST_STATUS));
''')
FAKE_NORMALIZER = textwrap.dedent('''\
    #!/usr/bin/env python3
    import json, os, sys
    with open(os.environ['BD_TEST_NORMALIZER_LOG'], 'a') as handle:
        handle.write(json.dumps(sys.argv[1:]) + '\\n')
    if os.environ.get('BD_TEST_REPAIR'):
        from pathlib import Path
        for store in sys.argv[1:]:
            for entry in Path(store).rglob('*'):
                if entry.is_file():
                    entry.chmod(0o660)
    print('private normalizer output')
    print('private normalizer diagnostic', file=sys.stderr)
    raise SystemExit(73)
''')


class BdWrapperTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory(prefix='bd-wrapper-test-')
        self.root = Path(self.tmp.name)
        self.workspace = self.root / 'first project'
        self.other = self.root / 'second project'
        for workspace in (self.workspace, self.other):
            store = workspace / '.beads'
            store.mkdir(parents=True)
            (store / 'config.yaml').write_text('issue-prefix: fixture\n')
            (store / 'metadata.json').write_text(json.dumps({'backend': 'dolt', 'database': 'embeddeddolt'}))
        self.fake_bd = self.root / 'fake bd.js'
        self.normalizer = self.root / 'fake normalizer'
        self.args_log = self.root / 'args.json'
        self.normalizer_log = self.root / 'normalizer.jsonl'
        self.fake_bd.write_text(FAKE_BD)
        self.normalizer.write_text(FAKE_NORMALIZER)
        self.normalizer.chmod(0o755)
        self.env = {k: v for k, v in os.environ.items() if not k.startswith('BEADS_')}
        self.env.update(BD_REAL=str(self.fake_bd), BEADS_NORMALIZER=str(self.normalizer),
                        BD_TEST_ARGS=str(self.args_log), BD_TEST_NORMALIZER_LOG=str(self.normalizer_log))

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def run_wrapper(self, status: int, *args: str) -> subprocess.CompletedProcess[str]:
        self.env['BD_TEST_STATUS'] = str(status)
        return subprocess.run([str(SCRIPT), *args], cwd=self.workspace, env=self.env,
                              capture_output=True, text=True, check=False)

    def normalizer_calls(self) -> list[list[str]]:
        if not self.normalizer_log.exists():
            return []
        return [json.loads(line) for line in self.normalizer_log.read_text().splitlines()]

    def test_preserves_arguments_streams_status_and_ignores_normalizer_failure(self) -> None:
        args = ['ready', '--json', 'value with spaces', '', '$literal']
        self.env.update(BD_TEST_STDOUT='stdout without newline', BD_TEST_STDERR='stderr without newline')
        for status in (0, 19):
            with self.subTest(status=status):
                self.normalizer_log.unlink(missing_ok=True)
                result = self.run_wrapper(status, *args)
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertEqual(result.stdout, self.env['BD_TEST_STDOUT'])
                self.assertEqual(result.stderr, self.env['BD_TEST_STDERR'])
                self.assertEqual(json.loads(self.args_log.read_text()), args)
                self.assertEqual(self.normalizer_calls(), [[str(self.workspace / '.beads')]] * 2)

    def test_store_selection_and_flaglike_note_values(self) -> None:
        selectors = [
            ['-C', str(self.other)], ['-C' + str(self.other)], ['-C=' + str(self.other)],
            ['-qC' + str(self.other)], ['--directory', str(self.other)],
            ['-q=false', '-C', str(self.other)], ['-q=true', '-C', str(self.other)],
            ['--directory=' + str(self.other)],
            ['--db', str(self.other / '.beads' / 'embeddeddolt')],
            ['--db=' + str(self.other / '.beads' / 'embeddeddolt')],
            ['-C', str(self.other), '--db', '.beads/embeddeddolt'],
        ]
        for selector in selectors:
            for before in (True, False):
                with self.subTest(selector=selector, before=before):
                    self.normalizer_log.unlink(missing_ok=True)
                    args = [*selector, 'update'] if before else ['update', *selector]
                    args += ['fixture-1', '--notes', '--directory=' + str(self.workspace)]
                    result = self.run_wrapper(0, *args)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(json.loads(self.args_log.read_text()), args)
                    self.assertEqual(self.normalizer_calls(), [[str(self.other / '.beads')]] * 2)

    def test_beads_dir_and_argument_terminator(self) -> None:
        self.env['BEADS_DIR'] = str(self.other / '.beads')
        result = self.run_wrapper(0, 'ready', '--', '--directory', str(self.workspace))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.normalizer_calls(), [[str(self.other / '.beads')]] * 2)

    def test_resolution_failure_never_invokes_normalizer_defaults(self) -> None:
        for output, status in [('invalid json', '0'), ('{}', '0'), ('{"path": "relative"}', '0'),
                               (json.dumps({'path': str(self.workspace / '.beads')}), '3')]:
            with self.subTest(output=output, status=status):
                self.env.update(BD_TEST_WHERE_OUTPUT=output, BD_TEST_WHERE_STATUS=status)
                result = self.run_wrapper(17, 'ready')
                self.assertEqual(result.returncode, 17)
                self.assertEqual(result.stdout, '')
                self.assertEqual(result.stderr, '')
                self.assertEqual(self.normalizer_calls(), [])

    def test_temp_file_rename_repairs_unlisted_store_and_leaves_second_store_untouched(self) -> None:
        manifest = self.other / '.beads' / 'manifest'
        untouched = self.workspace / '.beads' / 'manifest'
        for file in (manifest, untouched):
            file.write_text('original manifest\n')
            file.chmod(0o600)
        inode = manifest.stat().st_ino
        self.env.update(BD_TEST_REPAIR='1', BD_TEST_MANIFEST=str(manifest))
        result = self.run_wrapper(19, 'update', 'fixture-1', '-C', str(self.other))
        self.assertEqual(result.returncode, 19, result.stderr)
        self.assertNotEqual(manifest.stat().st_ino, inode)
        self.assertEqual(manifest.read_text(), 'new manifest\n')
        self.assertEqual(stat.S_IMODE(manifest.stat().st_mode), 0o660)
        self.assertEqual(stat.S_IMODE(untouched.stat().st_mode), 0o600)
        self.assertEqual(untouched.read_text(), 'original manifest\n')
        self.assertEqual(self.normalizer_calls(), [[str(self.other / '.beads')]] * 2)

    @unittest.skipUnless(REAL_BD.is_file(), 'installed bd CLI unavailable')
    def test_native_cli_resolution_uses_disposable_store(self) -> None:
        self.env['BD_REAL'] = str(REAL_BD)
        for selector in (['-C', str(self.other)],
                         ['--db', str(self.other / '.beads' / 'embeddeddolt')],
                         ['-q=false', '-C', str(self.other)],
                         ['-q=true', '-C', str(self.other)]):
            with self.subTest(selector=selector):
                self.normalizer_log.unlink(missing_ok=True)
                result = self.run_wrapper(0, *selector, 'where', '--json')
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(json.loads(result.stdout)['path'], str(self.other / '.beads'))
                self.assertEqual(self.normalizer_calls(), [[str(self.other / '.beads')]] * 2)

    def test_suppresses_only_existing_shared_mode_warning(self) -> None:
        warning = 'Warning: /tmp/project/.beads has permissions 0770 (recommended: 0700). Run: chmod 700 /tmp/project/.beads\n'
        other = 'Warning: keep this warning\nWarning: /tmp/project/.beads has permissions 0755 (recommended: 0700). Run: chmod 700 /tmp/project/.beads\n'
        self.env['BD_TEST_STDERR'] = warning + other
        result = self.run_wrapper(0, 'ready')
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stderr, other)


if __name__ == '__main__':
    unittest.main()
