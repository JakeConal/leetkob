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
- `cmd/worker` — the separate judging worker. Task 2.1 provides the
  shared PostgreSQL store and transactions. Job leasing arrives with
  task 4.2; until then the worker starts, idles and shuts down cleanly.
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

## Sandbox

Untrusted C++17 compilation and execution go through
`internal/sandbox`. The fixed image, the nine attack fixtures, and the
exact `docker build` / `docker create` lines the future `pa3` tag job must
run are in [`sandbox/README.md`](sandbox/README.md). This package does not
map student verdicts (task 3.5) and it does not change CI (task 8.1).

```bash
cd apps/backend
CGO_ENABLED=0 go test ./internal/sandbox/... -count=1 -timeout 20m
```

`PORT` selects the listen port (default 8080, validated 1–65535). With
`DATABASE_URL` set, the API opens a bounded connection pool (max 4
connections), applies the additive schema, and health verifies the store.
Without it, the process runs storeless and health reports process state
only. The worker uses the same `DATABASE_URL` when job leasing lands.

## Database

PostgreSQL 18 is the one durable store. Local connections bind to
loopback. Set `DATABASE_URL` to a URL the API can open, for example
`postgres://postgres:postgres@127.0.0.1:5432/leetkob?sslmode=disable`.

On startup the API calls `Store.Initialize`, which applies
[`db/001_init.sql`](db/001_init.sql). The script is idempotent: it
creates missing tables and indexes and records `001_init` once in
`schema_migrations`. Running it again does not drop tables, columns, or
rows and does not rewrite existing records. There is no destructive
migration. A change to an existing column would be a new additive script,
not an edit that removes data.

Initialization does not seed accounts, classes, or the marked lab. Those
stay empty until a later product command writes them. The script has no
triggers. Authorization, deadlines, counted selection, lease fencing, and
budget decisions stay in Go and commit or roll back through the
transaction port (`Begin`, `Commit`, `Rollback`). An error before commit,
or a process exit before commit, leaves the interrupted write invisible.
A row that was committed is readable from a new store connection after
restart.

Integration tests read `POSTGRES_URL` when it is set (a superuser URL,
because the test creates and removes a disposable database). Otherwise
they use `postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable`
when something is already accepting connections on `127.0.0.1:5432`.
When neither is available the integration test skips and says so. A skip
is not a passing durability check. The test database name starts with
`leetkob_it_` and is removed afterward; the test does not empty any
other database.

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
