"""Release metadata must follow actual checks on the same app, never a status flag."""

import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch

from tools.package_desktop_release import macos_signing_state


class ReleaseSigningTests(unittest.TestCase):
    def command(self, developer=True):
        return subprocess.CompletedProcess(
            [],
            0,
            "",
            "Authority=Developer ID Application: Invented" if developer else "Signature=adhoc",
        )

    def test_integrated_submission_runs_the_complete_gate_without_duplicate_ticket_request(self):
        app = Path("Invented.app")
        with (
            patch("tools.package_desktop_release.subprocess.run", return_value=self.command()),
            patch("tools.package_desktop_release.notarize") as submit,
            patch("tools.package_desktop_release.ticket_check") as ticket,
        ):
            self.assertEqual(macos_signing_state(app, True), "Developer ID, notarized and stapled")
            submit.assert_called_once_with(app)
            ticket.assert_not_called()

    def test_submission_failure_cannot_produce_signed_metadata(self):
        with (
            patch("tools.package_desktop_release.subprocess.run", return_value=self.command()),
            patch(
                "tools.package_desktop_release.notarize",
                side_effect=RuntimeError("Apple verification failed"),
            ),
        ):
            with self.assertRaisesRegex(RuntimeError, "Apple verification failed"):
                macos_signing_state(Path("Invented.app"), True)

    def test_standalone_packaging_still_requires_ticket_validation(self):
        with (
            patch("tools.package_desktop_release.subprocess.run", return_value=self.command()),
            patch("tools.package_desktop_release.notarize") as submit,
            patch(
                "tools.package_desktop_release.ticket_check",
                side_effect=RuntimeError("Invalid ticket"),
            ) as ticket,
        ):
            with self.assertRaisesRegex(RuntimeError, "Invalid ticket"):
                macos_signing_state(Path("Invented.app"), False)
            ticket.assert_called_once()
            submit.assert_not_called()

    def test_flag_cannot_convert_an_adhoc_signature_into_developer_id(self):
        with (
            patch("tools.package_desktop_release.subprocess.run", return_value=self.command(False)),
            patch("tools.package_desktop_release.notarize") as submit,
        ):
            with self.assertRaisesRegex(RuntimeError, "Developer ID"):
                macos_signing_state(Path("Invented.app"), True)
            self.assertEqual(
                macos_signing_state(Path("Invented.app"), False), "ad-hoc, not notarized"
            )
            submit.assert_not_called()

    def test_corrupt_signature_fails_before_any_submission(self):
        with (
            patch(
                "tools.package_desktop_release.subprocess.run",
                side_effect=subprocess.CalledProcessError(1, ["codesign"]),
            ),
            patch("tools.package_desktop_release.notarize") as submit,
        ):
            with self.assertRaises(subprocess.CalledProcessError):
                macos_signing_state(Path("Invented.app"), True)
            submit.assert_not_called()
