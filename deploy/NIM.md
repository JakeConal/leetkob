# Configure the deployed NVIDIA NIM worker

Run this setup yourself in a trusted terminal. If a token was pasted into a chat,
revoke it in the NVIDIA account and generate a replacement before proceeding.
Keep the replacement out of chat, Git, screenshots and Azure portal script text.

Prerequisites: Python 3, OpenSSL and Azure CLI signed in with access to
`vm-leetkob-pa3` in `jenkins_group`, subscription
`21ae08a0-914e-4a23-9985-439f8ac51417`.

```bash
python3 deploy/configure-nim.py
```

Enter the replacement token at the hidden terminal prompt. Alternatively, create
a private local `.env` with `NVIDIA_NIM_API_KEY` and run
`python3 deploy/configure-nim.py --env-file /absolute/path/to/.env`.
The repository ignores `.env`; keep its permissions at `0600` and keep it out
of container build contexts. This local input file is never copied to the VM.

The helper obtains a temporary RSA public key from the VM and encrypts only the
selected token using RSA-OAEP/SHA-256. Azure Run Command receives ciphertext.
The VM decrypts it into `/etc/leetkob/model-token`, owned by uid/gid `65534`
with mode `0400`, and removes the temporary transport key. Only a success marker
and file metadata return to the terminal. The helper installs the credential;
activation and a real model result must still be verified.

After the reviewed worker image has been built on the VM, activate it using
Azure Run Command → RunShellScript:

```bash
set -eu
cd /opt/leetkob
docker compose --env-file /etc/leetkob/deploy.env \
  -f deploy/docker-compose.yml -f deploy/docker-compose.model.yml \
  up -d --no-deps --wait worker
```

Use both Compose files for subsequent worker updates or restarts. The base
configuration defaults to model-off. The overlay mounts the token into the
trusted worker only; the isolated compiler and analyzer receive no provider
credential, application environment or Docker socket. No unit or contract test
calls a real provider. Verify deployment with an authorized synthetic submission
and actual request-ID/token/audit records before claiming live integration.

Offline transfer tests use a disposable local RSA key and a synthetic token:

```bash
python3 -m unittest discover -s deploy -p test_configure_nim.py -v
```
