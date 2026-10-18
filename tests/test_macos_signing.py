"""Credential handling checks; these never call Apple services or a real keychain."""

import base64
import os
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from tools.macos_signing import decode_material, prepare, run, signing_identity


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


class SigningMaterialTests(unittest.TestCase):
    def test_muesli_base64_material_and_data_urls(self):
        content = b"invented certificate bytes"
        encoded = base64.b64encode(content).decode()
        self.assertEqual(decode_material(encoded + "\n"), content)
        self.assertEqual(decode_material("data:application/x-pkcs12;base64," + encoded), content)
        with self.assertRaisesRegex(RuntimeError, "base64"):
            decode_material("https://credentials.invalid/private-certificate.p12")

    def test_identity_is_unambiguous_and_matches_the_configured_team(self):
        fingerprint = "A" * 40
        listing = f'  1) {fingerprint} "Developer ID Application: Invented Name (ABCDE12345)"\n     1 valid identities found\n'
        self.assertEqual(signing_identity(listing, "ABCDE12345"), fingerprint)
        with self.assertRaisesRegex(RuntimeError, "one valid"):
            signing_identity(listing, "WRONG12345")
        with self.assertRaisesRegex(RuntimeError, "one valid"):
            signing_identity(listing + listing, "ABCDE12345")
