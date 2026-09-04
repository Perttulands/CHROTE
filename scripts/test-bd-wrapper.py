#!/usr/bin/env python3
"""Behavior tests for CHROTE's vendored Beads launcher."""

from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest


SCRIPT = Path(
    os.environ.get(
        "CHROTE_BD_WRAPPER_UNDER_TEST",
        Path(__file__).parents[1] / "bin" / "bd",
    )
)

FAKE_BD = textwrap.dedent(
    """\
    #!/usr/bin/env node
    const fs = require("fs");

    fs.writeFileSync(
      process.env.BD_WRAPPER_TEST_ARGS,
      JSON.stringify(process.argv.slice(2)),
    );
    if (process.env.BD_WRAPPER_TEST_STDERR) {
      process.stderr.write(process.env.BD_WRAPPER_TEST_STDERR);
    }
    process.exit(Number(process.env.BD_WRAPPER_TEST_STATUS));
    """
)

FAKE_NORMALIZER = textwrap.dedent(
    """\
    #!/usr/bin/env python3
    import os

    with open(
        os.environ["BD_WRAPPER_TEST_NORMALIZER_LOG"],
        "a",
        encoding="utf-8",
    ) as handle:
        handle.write("normalized\\n")
    raise SystemExit(73)
    """
)


class BdWrapperTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory(prefix="chrote-bd-wrapper-test-")
        self.root = Path(self.tmp.name)
        self.fake_bd = self.root / "fake bd.js"
        self.normalizer = self.root / "fake normalizer"
        self.args_log = self.root / "args.json"
        self.normalizer_log = self.root / "normalizer.log"
        self.fake_bd.write_text(FAKE_BD, encoding="utf-8")
        self.normalizer.write_text(FAKE_NORMALIZER, encoding="utf-8")
        self.normalizer.chmod(0o755)

        self.env = os.environ.copy()
        self.env["BD_REAL"] = str(self.fake_bd)
        self.env["BEADS_NORMALIZER"] = str(self.normalizer)
        self.env["BD_WRAPPER_TEST_ARGS"] = str(self.args_log)
        self.env["BD_WRAPPER_TEST_NORMALIZER_LOG"] = str(self.normalizer_log)
        self.env["BD_WRAPPER_TEST_STDERR"] = ""

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def run_wrapper(
        self,
        status: int,
        *args: str,
    ) -> subprocess.CompletedProcess[str]:
        self.env["BD_WRAPPER_TEST_STATUS"] = str(status)
        return subprocess.run(
            [str(SCRIPT), *args],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
        )

    def normalizer_calls(self) -> list[str]:
        if not self.normalizer_log.exists():
            return []
        return self.normalizer_log.read_text(encoding="utf-8").splitlines()

    def test_forwards_arguments_and_normalizes_before_and_after(self) -> None:
        expected = ["ready", "--json", "value with spaces", "", "$literal"]

        result = self.run_wrapper(0, *expected)

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(self.args_log.read_text(encoding="utf-8")), expected)
        self.assertEqual(self.normalizer_calls(), ["normalized", "normalized"])

    def test_preserves_failure_status(self) -> None:
        result = self.run_wrapper(19, "prime", "failure value")

        self.assertEqual(result.returncode, 19, result.stderr)
        self.assertEqual(self.normalizer_calls(), ["normalized", "normalized"])

    def test_suppresses_only_the_intentional_shared_store_warning(self) -> None:
        self.env["BD_WRAPPER_TEST_STDERR"] = (
            "Warning: /srv/chrote/.beads has permissions 0770 "
            "(recommended: 0700). Run: chmod 700 /srv/chrote/.beads\n"
            "Warning: keep this unrelated warning\n"
        )

        result = self.run_wrapper(0, "ready")

        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stderr, "Warning: keep this unrelated warning\n")

    def test_preserves_other_permission_modes(self) -> None:
        warning = (
            "Warning: /tmp/project/.beads has permissions 0755 "
            "(recommended: 0700). Run: chmod 700 /tmp/project/.beads\n"
        )
        self.env["BD_WRAPPER_TEST_STDERR"] = warning

        result = self.run_wrapper(0, "ready")

        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stderr, warning)


if __name__ == "__main__":
    unittest.main()
