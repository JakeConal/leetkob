# LeeTKOB Go backend (PA#3)

The PA#3 production commands for LeeTKOB. Three separate processes own
three separate responsibilities, per the assignment: the API never judges
inline, the worker drains the durable queue, and the checker marks a
dataset independently of the product.

- `cmd/api` — production API on `/v2` (routes land with issue #38 tasks
  2.2–2.4, 4 and 6, built against
  [`docs/contract/pa3-openapi.json`](../../docs/contract/pa3-openapi.json)).
  Currently serves `GET /healthz`. The PA#2 compatibility API
  (`apps/api`, unprefixed `/labs/{labId}/publish`) keeps running
  separately and is never registered against production storage.
- `cmd/worker` — the separate judging worker. The durable PostgreSQL
  queue adapter arrives with tasks 2.1/4.2; until then it starts, idles
  and shuts down cleanly.
- `cmd/checker` — the dataset-independent marking command
  (`--data <dir> --predictions <file>`). Argument validation and the
  `MODEL_OFF=1` selection point are pinned; generic discovery lands with
  task 7.1. Exit codes: `2` invalid invocation, `0`/`1` valid run
  outcomes decided by the unmodified scorer.

## Toolchain and module pins

| Component             | Pin                                             | License      |
| --------------------- | ----------------------------------------------- | ------------ |
| Go                    | 1.27.4 (snap, `go 1.27.4` in `go.mod`)          | BSD-3-Clause |
| `jackc/pgx/v5`        | v5.11.0                                         | MIT          |
| `jackc/pgpassfile`    | v1.0.0 (transitive)                             | MIT          |
| `jackc/pgservicefile` | v0.0.0-20240606120523-5a60cdf6a761 (transitive) | MIT          |
| `jackc/puddle/v2`     | v2.2.2 (transitive)                             | MIT          |
| `golang.org/x/crypto` | v0.59.0 (Argon2id)                              | BSD-3-Clause |
| `golang.org/x/sync`   | v0.25.0 (transitive)                            | BSD-3-Clause |
| `golang.org/x/sys`    | v0.50.0 (transitive)                            | BSD-3-Clause |
| `golang.org/x/text`   | v0.44.0 (transitive)                            | BSD-3-Clause |

Exact `go.sum` entries and the resolved image digests are recorded during
the clean build qualification in CI (task 1.4/8.1). No provider SDK,
telemetry package or JWT library is used: provider transport is the Go
standard library behind an injected port, and sessions are opaque
server-side tokens.

## Startup

```bash
cd apps/backend
go build ./...
PORT=8080 DATABASE_URL=postgres://... go run ./cmd/api
go run ./cmd/worker
go run ./cmd/checker --data <dataset-dir> --predictions <output.jsonl>
```

`PORT` selects the listen port (default 8080, validated 1–65535). With
`DATABASE_URL` set, the API opens a bounded connection pool (max 4
connections) and health verifies the store; without it, the process runs
storeless and health reports process state only. The worker uses the same
`DATABASE_URL` when its queue adapter lands.

## Durable store (PostgreSQL 18)

The API, worker and integration tests share one additive schema. The
image is pinned to `postgres@sha256:eb63372c1e05c755f55fa30cbd07aecc443676fc6c0fa90b9002d01537841e1a`
(PostgreSQL 18.7, Debian `18.7-1.pgdg13+5`, resolved 2026-11-23).

Start a loopback-only database:

```bash
docker run -d --name leetkob-pg \
  -e POSTGRES_PASSWORD=leetkob-dev \
  -p 127.0.0.1:5432:5432 \
  postgres@sha256:eb63372c1e05c755f55fa30cbd07aecc443676fc6c0fa90b9002d01537841e1a

export DATABASE_URL='postgres://postgres:leetkob-dev@127.0.0.1:5432/postgres?sslmode=disable'
```

`cmd/api` applies the additive migrations automatically at startup and
records each version in `schema_migrations`; re-running is a no-op. Every
migration is additive (`CREATE ... IF NOT EXISTS`): there is **no
destructive migration path** in this repository, and recovery from a bad
schema never means dropping or rewriting durable data. The migrations
live in `internal/adapters/postgres/migrations` and are embedded in the
binary.

The integration suite provisions a disposable database per test: it
starts a temporary pinned PostgreSQL container when Docker is available,
or uses `TEST_DATABASE_URL` when pointed at an existing instance.
`TEST_POSTGRES_IMAGE` may override the image for local disposable runs
only.

## Synthetic assessment accounts

```bash
DATABASE_URL='postgres://postgres:leetkob-dev@127.0.0.1:5432/postgres?sslmode=disable' \
  go run ./cmd/provision
```

The command creates the five synthetic accounts, two classes and their
memberships: one admin (no class), `ta1` and `ta2` in separate classes,
and two students in the first TA's class. It is idempotent — a repeated
run never recreates an account or resets a password — and it never
creates the marked lab or any submission. Newly created passwords are
printed exactly once (only Argon2id hashes are stored).

## Sessions and auth routes

With `DATABASE_URL` configured, `cmd/api` serves:

| Route                  | Behaviour                                                                                                                                                                                                                                            |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `POST /v2/auth/login`  | 8-hour opaque session in an HttpOnly, host-only, SameSite=Lax cookie and a per-session CSRF token in the body. Generic 401 on any credential failure; durable throttling (10/min per address, 10/15 min per account) returns 429 with `Retry-After`. |
| `GET /v2/auth/me`      | Safe current user plus current class memberships from server-owned state.                                                                                                                                                                            |
| `POST /v2/auth/logout` | Requires `X-CSRF-Token`; revokes the session and clears the cookie.                                                                                                                                                                                  |

`WEB_ORIGIN` (default `http://127.0.0.1:3000`) rejects differing Origins
on mutations, and `SECURE_COOKIES=1` marks cookies `Secure` on deployed
HTTPS. Neither raw session tokens nor CSRF proofs are ever stored or
logged — only SHA-256 digests.

## Health

`GET /healthz` returns `200 {"status":"ok"}` with
`Cache-Control: no-store`. When a store is configured and unreachable, it
returns the contract's `503` `unavailable` problem
(`application/problem+json`, type
`https://leetkob.example/problems/unavailable`) — see
[`docs/contract/fixtures/pa3-problems.json`](../../docs/contract/fixtures/pa3-problems.json).
Non-GET methods receive a `405` problem response.

## Shutdown

All three commands install `SIGINT`/`SIGTERM` handlers. The API drains
in-flight requests for up to 10 seconds before closing; the worker stops
without abandoning any durable state (jobs remain leased-with-expiry once
the queue adapter exists); the checker exits with its documented code.
A clean API shutdown logs `api shut down cleanly`.
