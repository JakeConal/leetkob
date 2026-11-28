"""Offline checks: use synthetic tokens and a disposable local RSA key only."""

import base64
import contextlib
import importlib.util
import io
from pathlib import Path
import re
import subprocess
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("configure_nim", Path(__file__).with_name("configure-nim.py"))
setup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(setup)


class ConfigureNimTest(unittest.TestCase):
    def test_azure_receives_ciphertext_and_vm_can_decrypt_exact_token(self):
        synthetic = "nvapi-" + "synthetic_fixture_" * 3
        scripts = []
        with tempfile.TemporaryDirectory() as temporary:
            private = Path(temporary) / "private.pem"
            subprocess.run([
                "openssl", "genpkey", "-algorithm", "RSA", "-pkeyopt", "rsa_keygen_bits:3072",
                "-out", str(private),
            ], check=True, capture_output=True)
            public = subprocess.run([
                "openssl", "pkey", "-in", str(private), "-pubout",
            ], check=True, capture_output=True, text=True).stdout

            def remote(_command, script, _directory):
                scripts.append(script)
                self.assertNotIn(synthetic, script)
                if len(scripts) == 1:
                    return "TRANSFER_DIR=/run/leetkob-nim-fixture\n" + public
                encoded = re.search(r"printf '%s' '([A-Za-z0-9+/=]+)'", script)[1]
                decrypted = subprocess.run([
                    "openssl", "pkeyutl", "-decrypt", "-inkey", str(private),
                    "-pkeyopt", "rsa_padding_mode:oaep", "-pkeyopt", "rsa_oaep_md:sha256",
                    "-pkeyopt", "rsa_mgf1_md:sha256",
                ], input=base64.b64decode(encoded), check=True, capture_output=True).stdout
                self.assertEqual(decrypted, synthetic.encode())
                return "LEETKOB_NIM_INSTALLED\nmode=400 owner=65534 group=65534"

            output = io.StringIO()
            with patch.object(setup, "az_command", return_value=["fixture-az"]), \
                    patch.object(setup, "verify_az"), \
                    patch.object(setup, "read_token", return_value=synthetic.encode()), \
                    patch.object(setup, "run_remote", side_effect=remote), \
                    patch("sys.argv", ["configure-nim.py"]), contextlib.redirect_stdout(output):
                setup.main()
            self.assertEqual(len(scripts), 2)
            self.assertNotIn(synthetic, output.getvalue())

    def test_relocated_bundled_python_precedes_broken_path_launcher(self):
        with tempfile.TemporaryDirectory() as temporary:
            home = Path(temporary)
            python = home / ".local/share/leetkob-tools/azure-cli/opt/az/bin/python3"
            python.parent.mkdir(parents=True)
            python.touch()
            with patch.object(setup.Path, "home", return_value=home), \
                    patch.object(setup.shutil, "which", return_value="/broken/az"):
                self.assertEqual(setup.az_command(), [str(python), "-m", "azure.cli"])

    def test_system_cli_fallback_when_no_bundle_exists(self):
        with tempfile.TemporaryDirectory() as temporary:
            with patch.object(setup.Path, "home", return_value=Path(temporary)), \
                    patch.object(setup.shutil, "which", return_value="/usr/bin/az"):
                self.assertEqual(setup.az_command(), ["/usr/bin/az"])
            with patch.object(setup.Path, "home", return_value=Path(temporary)), \
                    patch.object(setup.shutil, "which", return_value=None):
                with self.assertRaisesRegex(RuntimeError, "Azure CLI is required"):
                    setup.az_command()

    def test_cli_preflight_hides_raw_errors_and_output(self):
        for failure in (
            FileNotFoundError("private-local-detail"),
            subprocess.TimeoutExpired(["private-local-detail"], 30),
        ):
            with self.subTest(failure=type(failure).__name__), \
                    patch.object(setup.subprocess, "run", side_effect=failure):
                with self.assertRaisesRegex(RuntimeError, "Azure CLI could not start") as error:
                    setup.verify_az(["fixture-az"])
                self.assertNotIn("private-local-detail", str(error.exception))
        with patch.object(setup.subprocess, "run", return_value=subprocess.CompletedProcess(
            ["fixture-az"], 1, stdout="private-local-detail", stderr="private-local-detail",
        )):
            with self.assertRaisesRegex(RuntimeError, "Azure CLI could not start") as error:
                setup.verify_az(["fixture-az"])
            self.assertNotIn("private-local-detail", str(error.exception))

    def test_cli_failure_precedes_secret_prompt(self):
        with patch.object(setup, "az_command", return_value=["fixture-az"]), \
                patch.object(setup, "verify_az", side_effect=RuntimeError("Azure CLI could not start.")), \
                patch.object(setup, "read_token") as read_token, \
                patch("sys.argv", ["configure-nim.py"]):
            with self.assertRaisesRegex(RuntimeError, "Azure CLI could not start"):
                setup.main()
            read_token.assert_not_called()

    def test_env_is_parsed_as_data_and_ambiguous_credentials_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            env = Path(temporary) / ".env"
            expected = "nvapi-" + "synthetic_fixture_" * 2
            env.write_text("NVIDIA_NIM_API_KEY='" + expected + "' # comment\nUNRELATED=ignored\n")
            self.assertEqual(setup.read_token(str(env)), expected.encode())
            env.write_text("NVIDIA_NIM_API_KEY='" + expected + "'\nNIM_API_KEY='different'\n")
            with self.assertRaisesRegex(RuntimeError, "unambiguous"):
                setup.read_token(str(env))

    def test_noninteractive_prompt_and_invalid_token_do_not_leak_input(self):
        with patch("sys.stdin.isatty", return_value=False):
            with self.assertRaisesRegex(RuntimeError, "interactive"):
                setup.read_token(None)
        with patch("sys.stdin.isatty", return_value=True), \
                patch.object(setup.getpass, "getpass", return_value="invalid-sensitive-input"):
            with self.assertRaisesRegex(RuntimeError, "unexpected format") as error:
                setup.read_token(None)
            self.assertNotIn("invalid-sensitive-input", str(error.exception))


if __name__ == "__main__":
    unittest.main()
