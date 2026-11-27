# Run locally

```bash
./scripts/run-local.sh
# equivalent: npm run dev:local
```

Open **http://127.0.0.1:3000/live**. The script runs the Go API on port
8080, a separate judging worker, and Next.js in development mode on your
machine. Docker supplies PostgreSQL on port 5545 and the existing isolated
C++ execution/analyzer images. It does not build application Docker images.

Prerequisites: Linux/WSL, Bash, Node.js 22.13+, npm, Go 1.27.4, and a running
Docker Engine with Compose. The launcher detects the snap Go installation if
the default `go` is older; `LOCAL_GO_BIN` can select another installation.
It runs `npm ci --ignore-scripts` only if workspace dependencies are missing
and builds the sandbox/analyzer images only if they are not already present.
Those first-time setup steps need network access.

The default seed provisions five synthetic accounts and two classes, creates
eight labs through the API, and submits five C++ fixtures through the web
proxy. Passwords and direct lab links are saved in
**`.local-dev/v2/state.json`** (gitignored, mode 0600). No real student data is used.

| Email                                | Role               |
| ------------------------------------ | ------------------ |
| `admin@assessment.leetkob.test`      | Admin, no class    |
| `ta-a@assessment.leetkob.test`       | TA for Class A     |
| `ta-b@assessment.leetkob.test`       | TA for Class B     |
| `student-a1@assessment.leetkob.test` | Student in Class A |
| `student-a2@assessment.leetkob.test` | Student in Class A |

Seed scenarios cover compliant merge sort, a forbidden library call, a
prompt-injection comment, WA, CE, an empty queue, an expired deadline and
another class's lab. Repeated seeding reuses IDs and keeps existing passwords
and review decisions. Keep the state file with the database volume.

```bash
./scripts/run-local.sh --smoke  # also verify the real judge/check/review flow
./scripts/run-local.sh --smoke --resubmit  # rerun fixtures after a worker/checker change
./scripts/run-local.sh --no-seed
npm run seed:local              # reapply data while the launcher is running
npm run test:local
```

Press **Ctrl+C** in the launcher terminal to stop its API, worker, web process
and database container. The database volume and seed state remain for the next
run. Logs are under `.local-dev/v2/logs/`. A lock prevents duplicate launches,
and occupied API/web ports produce an error without stopping another process.

`--resubmit` creates a new submission for each fixture, preserving previous
results in history. Normal startup reuses the last seeded submissions.

Optional ports, used by both the launcher and its seed command:

```bash
LOCAL_WEB_PORT=3300 LOCAL_API_PORT=8180 LOCAL_POSTGRES_PORT=5545 ./scripts/run-local.sh
```

If re-seeding separately with custom ports, pass those same variables to
`npm run seed:local`. With `--no-seed`, no account manifest is created on a
fresh database; use the normal launch or seed command to provision accounts.

The API and web bind to loopback, and the database uses its own
`leetkob-local-v2` Compose project and volume. Application processes receive an
explicit local environment; existing model keys and production database URLs
are not forwarded. `MODEL_OFF=1` uses the current worker's offline semantic adapter, which returns `unsure` for TA
review, with no paid requests. Hard rules and C++ judging still use the real
isolated containers. The seed is for local testing, separate from coursework
evaluation data or deployment evidence. Browser interaction and visual checks
remain manual; `--smoke` tests HTTP end to end.

The v2 local state directory and Compose project are separate from earlier
prototype checkouts, whose database schema differs. Neither is migrated or
deleted by this launcher. The local analyzer is built from the existing
checker image and runs under the same sandbox controls as compilation.
