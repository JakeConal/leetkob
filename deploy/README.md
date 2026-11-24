# Repeatable PA#3 deployment (prepared, not provisioned)

This directory holds the deployment artifacts for the PA#3 stack. **Nothing
here has been deployed**: the owner chose to skip external provisioning on
2026-11-24, so there is no live URL and no claimed uptime. Everything below
is verified to the extent stated in "Local verification".

## Composition

| Service            | Image                                   | Notes                                                                 |
| ------------------ | --------------------------------------- | --------------------------------------------------------------------- |
| `postgres`         | `postgres@sha256:eb63372c…` (18.7)      | loopback-only port, named volume, `pg_isready` healthcheck            |
| `api`              | `deploy/Dockerfile.backend` (`BIN=api`) | runs the additive migrations on start, `/healthz`, non-root uid 65534 |
| `web`              | `deploy/Dockerfile.web`                 | Next.js app with `PA3_API_ORIGIN=http://api:8080` rewrites, non-root  |
| `worker`           | `deploy/Dockerfile.worker`              | judging worker with the libclang analyzer; needs a Docker daemon      |
| `caddy` (optional) | official Caddy                          | TLS termination for the public origin                                 |

```bash
cp deploy/.env.example deploy/.env      # fill synthetic values, never commit
docker compose -f deploy/docker-compose.yml config
docker compose -f deploy/docker-compose.yml up -d --build postgres api web
```

Then provision the five synthetic accounts **once** (no lab, no submissions):

```bash
cd apps/backend
ASSESSMENT_PASSWORDS='{"Admin":"…","TeachingAssistantA":"…","TeachingAssistantB":"…","StudentA1":"…","StudentA2":"…"}' \
DATABASE_URL='postgres://leetkob:…@127.0.0.1:5432/leetkob?sslmode=disable' \
go run ./cmd/provision
```

The command is idempotent: reruns never rewrite a password hash or recreate
a removed membership. Passwords are read from the environment only.

## Worker Docker constraint

The judging worker must reach a Docker daemon and its sandbox controller
bind-mounts a work directory into sibling execution containers. When the
worker itself runs in a container, the work path must be the **same
absolute path** on the host and inside the worker container, and the worker
needs `/var/run/docker.sock` (trusted controller only; no student code ever
reaches it):

```yaml
worker:
  build: { context: .., dockerfile: deploy/Dockerfile.worker }
  environment:
    DATABASE_URL: postgres://…@postgres:5432/leetkob?sslmode=disable
    TMPDIR: /srv/leetkob-work
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock
    - /srv/leetkob-work:/srv/leetkob-work
  restart: unless-stopped
```

`leetkob-cpp17:pa3` (the sandbox image) is built by the worker at startup
from `apps/backend/sandbox/cpp17` when missing; pre-building it during
provisioning avoids that first-run cost.

## Hosting proposal (free tier, Docker-capable)

| Option                                                          | Always free?     | Can run the Docker worker?                                       | Verdict                                                                                                      |
| --------------------------------------------------------------- | ---------------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| **Oracle Cloud Always Free VM** (Ampere/AMD, 1–4 OCPU, 6–24 GB) | Yes, no expiry   | Yes, full VM with Docker                                         | **Recommended**: one VM runs compose, Caddy, worker and database; $0 spend; keep it running until PA#4 marks |
| Google Cloud free `e2-micro`                                    | Yes (US regions) | Yes, small VM                                                    | Workable but 1 GB RAM is tight for Postgres + builds                                                         |
| Fly.io / Koyeb / Render / Railway free tiers                    | Limited          | **No** — no Docker-in-Docker, so the sandbox worker cannot judge | Not sufficient for this rubric                                                                               |
| GitHub Codespaces                                               | Free hours       | Timeout when idle                                                | Not always-on                                                                                                |

Recommended target: **one Oracle Cloud Always Free VM** with Docker and
compose, Caddy for TLS, the database bound to loopback, and a **spend
ceiling of USD 0**. Provisioning needs the owner's account and SSH access;
until then this remains a proposal, not a deployment.

## Health, restart, recovery, rollback

- **Health**: `GET /healthz` (API process + database ping) and the web
  `/live` page. Compose healthchecks restart unhealthy containers.
- **Restart**: worker restarts resume leased jobs after lease expiry
  (15 min) and keep a bounded infrastructure retry ceiling; exhausted jobs
  are visibly `failed`, never student verdicts.
- **Recovery**: all migrations are additive (`db/001_init.sql`,
  `CREATE … IF NOT EXISTS`); there is no destructive migration path. A
  crashed database keeps committed state; re-running any command is safe.
- **Rollback**: pin the previously deployed image digests and
  `docker compose up -d --no-build`; the schema stays compatible because it
  is additive. Keep `deploy/.env` out of Git.
- **Spend**: `MODEL_OFF=1` for the deployment worker (semantic checks stay
  unsure offline); no paid calls are made by the deployed stack. The
  per-lab cap remains USD 0.25 and the adapter refuses dispatch on unknown
  pricing.

## Local verification (2026-11-24)

| Check                                        | Result                                                              |
| -------------------------------------------- | ------------------------------------------------------------------- |
| `docker compose config`                      | valid                                                               |
| postgres + api containers start              | healthcheck passes, `/healthz` returns `{"status":"ok"}`            |
| `cmd/provision` against the compose database | five accounts + two classes created; rerun reported no changes      |
| web container builds and serves `/live`      | 200 (`next start` bound to 0.0.0.0 inside the container)            |
| login through the web rewrite                | 200 + `Set-Cookie` session for `student-a1@assessment.leetkob.test` |
| re-provisioning with different passwords     | reported no changes; the original password still signs in           |

Not verified locally: the public domain/TLS path (no domain), and the worker
in compose (needs the same-path host mount; the worker was exercised
directly against Docker elsewhere in the test suite). Both are stated
explicitly rather than claimed.
