"""Credential handling checks; these never call Apple services or a real keychain."""

import base64
import os
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from tools.macos_signing import (
    decode_material,
    prepare,
    public_check,
    run,
    signing_identity,
    ticket_check,
)


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

    def test_public_diagnostics_report_stapler_failure_but_reject_security_commands(self):
        with patch(
            "tools.macos_signing.subprocess.run",
            return_value=subprocess.CompletedProcess([], 65, "Ticket not found", ""),
        ) as command:
            with self.assertRaisesRegex(RuntimeError, "Ticket not found"):
                public_check(["xcrun", "stapler", "staple", "Example.app"])
            command.reset_mock()
            with self.assertRaisesRegex(RuntimeError, "restricted"):
                public_check(["security", "import", "secret"])
            command.assert_not_called()

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


class TicketTransportTests(unittest.TestCase):
    def test_lost_connection_retries_then_requires_success(self):
        failure = subprocess.CompletedProcess(
            [], 68, "", "Error Domain=NSURLErrorDomain Code=-1005"
        )
        success = subprocess.CompletedProcess([], 0, "Ticket validated", "")
        with (
            patch(
                "tools.macos_signing.subprocess.run", side_effect=[failure, failure, success]
            ) as command,
            patch("tools.macos_signing.time.sleep") as sleep,
        ):
            self.assertEqual(
                ticket_check(["xcrun", "stapler", "validate", "Invented.app"]), "Ticket validated"
            )
            self.assertEqual(command.call_count, 3)
            self.assertEqual([call.args[0] for call in sleep.call_args_list], [2, 4])

    def test_bad_ticket_is_fatal_without_retry(self):
        with (
            patch(
                "tools.macos_signing.subprocess.run",
                return_value=subprocess.CompletedProcess([], 68, "Invalid ticket", ""),
            ) as command,
            patch("tools.macos_signing.time.sleep") as sleep,
        ):
            with self.assertRaisesRegex(RuntimeError, "Invalid ticket"):
                ticket_check(["xcrun", "stapler", "validate", "Invented.app"])
            self.assertEqual(command.call_count, 1)
            sleep.assert_not_called()

    def test_persistent_transport_failure_is_bounded_and_still_fatal(self):
        with (
            patch(
                "tools.macos_signing.subprocess.run",
                return_value=subprocess.CompletedProcess([], 68, "NSURLErrorDomain Code=-1005", ""),
            ) as command,
            patch("tools.macos_signing.time.sleep") as sleep,
        ):
            with self.assertRaisesRegex(RuntimeError, "Code=-1005"):
                ticket_check(["xcrun", "stapler", "staple", "Invented.app"])
            self.assertEqual(command.call_count, 4)
            self.assertEqual([call.args[0] for call in sleep.call_args_list], [2, 4, 8])

    def test_retry_helper_rejects_credential_commands(self):
        with patch("tools.macos_signing.subprocess.run") as command:
            with self.assertRaisesRegex(RuntimeError, "restricted"):
                ticket_check(["security", "import", "invented-secret"])
            command.assert_not_called()
