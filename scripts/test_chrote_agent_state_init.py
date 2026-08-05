#!/usr/bin/env python3
"""Permission-contract tests for the locked-agent state provisioner."""
from __future__ import annotations

import os
import pwd
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]
PROVISIONER = REPO_ROOT / "scripts" / "chrote-agent-state-init"
TMPFILES = REPO_ROOT / "services" / "chrote-agent-state.conf"


class AgentStateProvisioningTests(unittest.TestCase):
    def test_config_and_receipt_domains_have_opposite_writers(self) -> None:
        for command in ("setfacl", "getfacl", "install"):
            if shutil.which(command) is None:
                self.skipTest(f"{command} is required for the state ownership contract")

        account = pwd.getpwuid(os.getuid()).pw_name
        group = subprocess.run(
            ["id", "-gn"], text=True, capture_output=True, check=True
        ).stdout.strip()
        root = Path(tempfile.mkdtemp(prefix="chrote-agent-state-"))
        units = root / "units"
        receipts = root / "receipts"
        try:
            result = subprocess.run(
                [
                    str(PROVISIONER),
                    "--service-user", account,
                    "--service-group", group,
                    "--target-user", account,
                    "--units-dir", str(units),
                    "--receipts-dir", str(receipts),
                ],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

            config_dir = units / account
            receipt_dir = receipts / account
            self.assertEqual(0o750, config_dir.stat().st_mode & 0o777)
            self.assertEqual(0o770, receipt_dir.stat().st_mode & 0o777)

            config_acl = subprocess.run(
                ["getfacl", "-cp", str(config_dir)],
                text=True,
                capture_output=True,
                check=True,
            ).stdout
            self.assertIn(f"user:{account}:r-x", config_acl)
            self.assertIn(f"default:user:{account}:r--", config_acl)
            self.assertIn("default:mask::r--", config_acl)

            config_file = config_dir / "agent.conf"
            config_file.write_text("typed=true\n")
            config_file.chmod(0o640)
            file_acl = subprocess.run(
                ["getfacl", "-cp", str(config_file)],
                text=True,
                capture_output=True,
                check=True,
            ).stdout
            self.assertIn(f"user:{account}:r--", file_acl)
            self.assertIn("mask::r--", file_acl)

            self.assertEqual(
                receipt_dir.stat().st_gid,
                units.stat().st_gid,
                "target-owned receipts must remain readable by the CHROTE service group",
            )
            self.assertTrue(receipt_dir.stat().st_mode & 0o2000, "receipt directory must be setgid")
            receipt_file = receipt_dir / "agent.receipt.json"
            receipt_file.write_text("{}\n")
            receipt_file.chmod(0o640)
            self.assertEqual(receipt_dir.stat().st_gid, receipt_file.stat().st_gid)

            source = PROVISIONER.read_text()
            self.assertIn('-o "$SERVICE_USER" -g "$SERVICE_GROUP"', source)
            self.assertIn('-o "$TARGET_USER" -g "$SERVICE_GROUP"', source)
        finally:
            shutil.rmtree(root, ignore_errors=True)

    def test_tmpfiles_provisions_only_the_two_base_domains(self) -> None:
        text = TMPFILES.read_text()
        self.assertIn("/srv/data/chrote/agent-units", text)
        self.assertIn("/srv/data/chrote/agent-receipts", text)
        self.assertNotIn("receipt.json", text)


if __name__ == "__main__":
    unittest.main()
