#!/usr/bin/env python3
"""Run manually in a trusted terminal; the token never enters Azure command logs."""

import argparse
import base64
import getpass
import json
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import tempfile


SUBSCRIPTION = "21ae08a0-914e-4a23-9985-439f8ac51417"
RESOURCE_GROUP = "jenkins_group"
VM = "vm-leetkob-pa3"


def az_command():
    # The relocated CLI's az script has an absolute build-time shebang.
    # Invoke its bundled interpreter directly, before considering PATH (which
    # may also contain a Windows launcher that cannot consume WSL temp paths).
    bundled = Path.home() / ".local/share/leetkob-tools/azure-cli/opt/az/bin/python3"
    if bundled.is_file():
        return [str(bundled), "-m", "azure.cli"]
    executable = shutil.which("az")
    if executable:
        return [executable]
    raise RuntimeError("Azure CLI is required; sign in to the hosting subscription first.")


def verify_az(command):
    # No credential is requested until we know the selected launcher can start.
    # Do not include raw CLI output, paths or subprocess exceptions in errors.
    message = "Azure CLI could not start. Check its Python interpreter and CLI installation."
    try:
        result = subprocess.run(
            command + ["version", "--output", "json", "--only-show-errors"],
            capture_output=True, text=True, check=False, timeout=30,
        )
    except (OSError, subprocess.TimeoutExpired):
        raise RuntimeError(message) from None
    if result.returncode:
        raise RuntimeError(message)


def run_remote(command, script, directory):
    script_path = directory / "command.sh"
    script_path.write_text(script)
    script_path.chmod(0o600)
    result = subprocess.run(
        command + [
            "vm", "run-command", "invoke", "--subscription", SUBSCRIPTION,
            "--resource-group", RESOURCE_GROUP, "--name", VM,
            "--command-id", "RunShellScript", "--scripts", "@" + str(script_path),
            "--query", "value[].message", "--output", "json", "--only-show-errors",
        ], capture_output=True, text=True, check=False,
    )
    if result.returncode:
        raise RuntimeError("Azure Run Command failed. Check account access and VM status in Azure.")
    try:
        return "\n".join(json.loads(result.stdout))
    except (ValueError, TypeError):
        raise RuntimeError("Azure returned an unexpected response.") from None


def read_token(env_file):
    if not env_file:
        if not sys.stdin.isatty():
            raise RuntimeError("Use an interactive trusted terminal to enter the replacement token.")
        value = getpass.getpass("Replacement NVIDIA NIM token (hidden): ").strip()
    else:
        values = []
        for line in Path(env_file).read_text().splitlines():
            match = re.fullmatch(r"\s*(?:export\s+)?(?:NVIDIA_NIM_API_KEY|NVIDIA_API_KEY|NIM_API_KEY)\s*=(.*)", line)
            if match:
                parts = shlex.split(match[1], comments=True)
                if len(parts) == 1:
                    values.append(parts[0])
        if len(set(values)) != 1:
            raise RuntimeError("The env file must contain one unambiguous NVIDIA NIM token.")
        value = values[0]
    if not re.fullmatch(r"nvapi-[A-Za-z0-9_-]{20,250}", value):
        raise RuntimeError("The NVIDIA NIM token has an unexpected format.")
    return value.encode()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--env-file", help="Optional private local .env; otherwise prompt without echo")
    arguments = parser.parse_args()
    command = az_command()
    verify_az(command)
    if not shutil.which("openssl"):
        raise RuntimeError("OpenSSL is required on this trusted computer.")
    token = read_token(arguments.env_file)
    with tempfile.TemporaryDirectory(prefix="leetkob-nim-") as temporary:
        directory = Path(temporary)
        response = run_remote(command, """set -eu
umask 077
transfer_dir=$(mktemp -d /run/leetkob-nim-XXXXXXXX)
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "$transfer_dir/key.pem" 2>/dev/null
printf 'TRANSFER_DIR=%s\\n' "$transfer_dir"
openssl pkey -in "$transfer_dir/key.pem" -pubout
""", directory)
        remote_directory = re.search(r"TRANSFER_DIR=(/run/leetkob-nim-[A-Za-z0-9]+)", response)
        public_key = re.search(r"-----BEGIN PUBLIC KEY-----[\s\S]*?-----END PUBLIC KEY-----", response)
        if not remote_directory or not public_key:
            raise RuntimeError("Could not obtain the VM's temporary transport public key.")
        public_path = directory / "public.pem"
        public_path.write_text(public_key[0] + "\n")
        encrypted = subprocess.run([
            "openssl", "pkeyutl", "-encrypt", "-pubin", "-inkey", str(public_path),
            "-pkeyopt", "rsa_padding_mode:oaep", "-pkeyopt", "rsa_oaep_md:sha256",
            "-pkeyopt", "rsa_mgf1_md:sha256",
        ], input=token, capture_output=True, check=False)
        del token
        if encrypted.returncode:
            raise RuntimeError("Token encryption failed; no token was sent to Azure.")
        ciphertext = base64.b64encode(encrypted.stdout).decode("ascii")
        remote = remote_directory[1]
        script = f"""set -eu
umask 077
transfer_dir='{remote}'
token_tmp=''
trap 'rm -rf -- "$transfer_dir"; if [ -n "$token_tmp" ]; then rm -f -- "$token_tmp"; fi' EXIT
install -d -m 0700 /etc/leetkob
token_tmp=$(mktemp /etc/leetkob/.model-token-XXXXXXXX)
printf '%s' '{ciphertext}' | base64 -d > "$transfer_dir/ciphertext"
openssl pkeyutl -decrypt -inkey "$transfer_dir/key.pem" -in "$transfer_dir/ciphertext" -out "$token_tmp" \\
  -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 -pkeyopt rsa_mgf1_md:sha256
chown 65534:65534 "$token_tmp"
chmod 0400 "$token_tmp"
mv -f -- "$token_tmp" /etc/leetkob/model-token
token_tmp=''
printf 'LEETKOB_NIM_INSTALLED\\n'
stat -c 'mode=%a owner=%u group=%g' /etc/leetkob/model-token
"""
        installed = run_remote(command, script, directory)
        if "LEETKOB_NIM_INSTALLED" not in installed:
            raise RuntimeError("VM installation did not report success; check Azure Run Command status.")
    print("Installed /etc/leetkob/model-token on vm-leetkob-pa3 (0400, uid/gid 65534).")
    print("The deployment worker can now be activated and verified separately.")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError) as error:
        # Never print file contents, CLI output, subprocess arguments or a token.
        print("NIM setup failed: " + (str(error) if isinstance(error, RuntimeError) else "check local prerequisites and file access."), file=sys.stderr)
        sys.exit(1)
