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
- `cmd/worker` — the separate judging worker. It leases durable PostgreSQL
  jobs, compiles and runs each submission only through
  `internal/sandbox`, and commits one fenced verdict set. Infrastructure
  failures retry a bounded number of times and then stay visibly failed;
  they are never turned into a student verdict. See
  [Worker judging and recovery](#worker-judging-and-recovery).
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
DATABASE_URL=postgres://... go run ./cmd/worker
go run ./cmd/checker --data <dataset-dir> --predictions <output.jsonl>
```

`PORT` selects the listen port (default 8080, validated 1–65535). With
`DATABASE_URL` set, the API opens a bounded connection pool (max 4
connections) and health verifies the store; without it, the process runs
storeless and health reports process state only. The worker requires the
same `DATABASE_URL`; it applies the additive migrations and leases durable
jobs. Its sandbox image defaults to `leetkob-sandbox:pa3` and can be pinned
with `WORKER_SANDBOX_IMAGE`.

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
leasing, finishes the current job when it can, and otherwise releases that
lease without counting an infrastructure failure; the checker exits with
its documented code. A clean API shutdown logs `api shut down cleanly`.
A clean worker shutdown logs `worker shut down cleanly`; an abrupt kill
leaves only a lease that expires and is re-leased under a higher fence.

## Worker judging and recovery

`cmd/worker` requires `DATABASE_URL`. It applies the same additive
migrations as the API, then leases durable jobs from PostgreSQL. Acceptance
commits the source, submission, counted selection, replay record and one
pending `judge` job atomically; the API never compiles or runs student code.

| Variable                  | Default               | Meaning                                                       |
| ------------------------- | --------------------- | ------------------------------------------------------------- |
| `DATABASE_URL`            | required              | PostgreSQL connection URL shared with the API                 |
| `WORKER_ID`               | `worker-1`            | Lease owner recorded in structured logs                       |
| `WORKER_SANDBOX_IMAGE`    | `leetkob-sandbox:pa3` | Fixed execution image; never a caller-supplied image          |
| `WORKER_BATCH`            | `1`                   | Maximum jobs leased per poll                                  |
| `WORKER_LEASE_SECONDS`    | `1800`                | Lease duration before another worker may recover the job      |
| `WORKER_MAX_FAILURES`     | `3`                   | Infrastructure failures before a job becomes visibly `failed` |
| `WORKER_POLL_SECONDS`     | `2`                   | Idle poll interval                                            |
| `WORKER_SHUTDOWN_SECONDS` | `30`                  | Bound for finishing, releasing and cleanup during shutdown    |

The worker enforces these recovery rules:

- `LeaseJobs` uses `FOR UPDATE SKIP LOCKED`, increments the attempt fence
  and assigns a lease deadline. Another worker cannot take an active lease.
- `CommitJudgeResult` only succeeds for the current attempt while its lease
  is live. A duplicate delivery or a late attempt after expiry returns
  `ErrStaleFence` and cannot replace or duplicate stored rows.
- Results are keyed by submission and per-test ID. A duplicate execution
  therefore leaves exactly one aggregate verdict and one row per test.
- The worker checks the submission lab, generation and source SHA-256
  against the leased job envelope before executing anything. A mismatch is
  an operational failure, never a verdict.
- Sandbox and store failures go through `FailJob`, which is bounded by
  `WORKER_MAX_FAILURES`. The exhausted job stays visibly failed and never
  becomes a student WA/RE.
- On `SIGTERM` the worker stops leasing. It commits a result that is already
  complete; otherwise it releases the lease to `pending` without counting a
  failure. If the process is killed, the lease simply expires.
- Structured JSON logs include `jobId`, `submissionId`, `attempt`, the
  verdict and per-test count. They never include source, hidden tests or
  credentials.

### Recorded stop/submit/restart exercise

The automated proof starts a disposable pinned PostgreSQL 18.7 database,
seeds a verification lab through the store, accepts a submission while no
worker is running, and then starts the real worker with the local
`leetkob-sandbox:pa3` image. It asserts one `done` job, one aggregate
verdict and exactly one stored row per test after a duplicate delivery and
a worker restart. A second test expires a lease, lets a replacement claim
the next attempt, and proves the old attempt's commit is refused. A third
test drives the real HTTP ingress while the worker is stopped, runs the
real worker, polls the safe result endpoint, and checks that another
student and another class's TA receive a concealed `404` while the owner
and assigned TA receive the judge verdicts.

Repeat it with:

```bash
cd apps/backend
PATH="/snap/go/current/bin:$PATH" GOTOOLCHAIN=local \
  go test ./internal/worker/ -count=1 -v \
  -run 'TestHTTPIngressToJudgedResultStaysOwnerPrivate|TestStopSubmitRestartStoresExactlyOneVerdictSet|TestLateAttemptAfterLeaseExpiryCannotCommit|TestTwoWorkersJudgeOneSubmissionExactlyOnce'
```

Recorded on 2026-11-23 against the pinned PostgreSQL image and
`leetkob-sandbox:pa3`:

```text
=== RUN   TestHTTPIngressToJudgedResultStaysOwnerPrivate
--- PASS: TestHTTPIngressToJudgedResultStaysOwnerPrivate (21.38s)
=== RUN   TestStopSubmitRestartStoresExactlyOneVerdictSet
--- PASS: TestStopSubmitRestartStoresExactlyOneVerdictSet (10.76s)
=== RUN   TestLateAttemptAfterLeaseExpiryCannotCommit
--- PASS: TestLateAttemptAfterLeaseExpiryCannotCommit (10.62s)
=== RUN   TestTwoWorkersJudgeOneSubmissionExactlyOnce
--- PASS: TestTwoWorkersJudgeOneSubmissionExactlyOnce (10.90s)
PASS
ok  	github.com/JakeConal/leetkob/backend/internal/worker	53.668s
```

The manual equivalent is:

1. Start PostgreSQL and export `DATABASE_URL` as shown above.
2. Provision the synthetic accounts and create a lab through the TA API.
3. Leave `cmd/worker` stopped. Sign in as the enrolled student and submit
   through `POST /v2/labs/{labId}/submissions`; the response is `202` with
   `state: "queued"` and the pending job is visible in `jobs`.
4. Start `cmd/worker` with the same `DATABASE_URL`.
5. Poll `GET /v2/submissions/{submissionId}` until `state` is `judged` (the
   web submission panel does this every second). The response then carries
   `judge.verdict` and per-test `{id, verdict, timeMs}` only.
6. Verify the stored effect:

   ```sql
   SELECT state, verdict, attempt FROM submissions s
   JOIN jobs j ON j.submission_id = s.id
   WHERE s.id = '<submission-id>';
   -- judged | AC | 1

   SELECT count(*) FROM submission_test_results
   WHERE submission_id = '<submission-id>';
   -- number of lab tests, exactly once
   ```

7. Send `SIGTERM` to the worker, restart it, and confirm the job attempt and
   result counts do not change. A duplicate commit with the same attempt
   reports a stale fence.

The tests skip only when neither `TEST_DATABASE_URL` nor Docker is
available, or when the real sandbox image is absent; the skip message names
the missing prerequisite. They never substitute a fake judge for the
stop/submit/restart claim.
