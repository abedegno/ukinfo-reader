"""Credential handling checks; these never call Apple services or a real keychain."""

import os
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from tools.macos_signing import prepare, run


class SigningCredentialsTests(unittest.TestCase):
    def test_subprocess_failure_never_echoes_credential_arguments_or_output(self):
        secret = "invented-secret-that-must-not-be-logged"
        with patch(
            "tools.macos_signing.subprocess.run",
            return_value=subprocess.CompletedProcess(["security", "-p", secret], 1, secret, secret),
        ):
            with self.assertRaises(RuntimeError) as caught:
                run(["security", "-p", secret])
        self.assertNotIn(secret, str(caught.exception))
        self.assertIn("security failed", str(caught.exception))

    def test_missing_credentials_fail_before_any_keychain_mutation(self):
        with tempfile.TemporaryDirectory() as directory:
            with (
                patch.dict(
                    os.environ, {"GITHUB_ACTIONS": "true", "RUNNER_TEMP": directory}, clear=True
                ),
                patch("tools.macos_signing.sys.platform", "darwin"),
                patch("tools.macos_signing.subprocess.run") as command,
            ):
                with self.assertRaisesRegex(RuntimeError, "six signing"):
                    prepare()
                command.assert_not_called()
