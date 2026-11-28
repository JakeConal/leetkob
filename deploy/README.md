# PA#3 deployment

The public service is deployed on Azure in `jenkins_group`:
<https://leetkob-pa3-21ae08a0.southeastasia.cloudapp.azure.com/live>.
It first became reachable on November 28, before the updated November 29,
22:00 Asia/Saigon deadline. It must remain available until PA#4 marks.

- [Azure runbook](AZURE.md): host configuration, confirmed student-credit funding,
  estimated USD 42.91/month baseline, USD 50 notification budget, recovery and rollback.
- [Private NVIDIA NIM setup](NIM.md): operator-only credential installation and
  bounded live worker overlay. Never render Compose configuration with secrets.
- [Evidence index](../PA3.md): accounts, actual deployment checks and recording.

The base composition runs PostgreSQL, API, web and the trusted worker. Caddy
terminates public HTTPS. Images and toolchains are pinned; persistent volumes
retain database and TLS state. Only ports 80/443 are internet-facing.

The trusted worker alone receives the Docker socket. Compiler and analyzer
containers are created through the qualified controller with no network,
application environment, provider key, Docker socket or host binds. Task files
are copied into those containers; a same-path host work mount is unnecessary.
The real-model overlay adds a private token file only to the trusted worker.

Use the private runtime configuration at `/etc/leetkob/deploy.env` on the VM.
For a separate synthetic local deployment, copy `.env.example` into an ignored
private file and replace the example values. Provision the five synthetic
accounts once with `cmd/provision`; rerunning it never resets passwords or
creates a marking lab. Follow the Azure runbook for the actual commands.

Health checks report readiness; Docker restart policies recover stopped
processes. Persistent leases recover interrupted work after expiry. Keep database
volumes during updates; do not run `down -v` or automatically reverse migrations.
Budget notifications do not impose an automatic USD 50 cutoff. The student
subscription spending limit remains enabled; no pay-as-you-go upgrade is used.
