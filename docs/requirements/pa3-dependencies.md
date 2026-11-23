# PA#3 dependency review

Revised November 20, 2026 after the owner requested alignment with the published
PA#3 rubric. Status: awaiting approval for the additions below. This does not
authorize a paid model request or public deployment.

PA#3 implements the required end-to-end flow in Go. The existing full-product
OpenSpec design remains the later-milestone plan; this inventory does not make
registration, refresh JWTs, Google sign-in, appeals, telemetry infrastructure,
CSV import, or frontend framework changes prerequisites for PA#3.

| Component                      | Proposed pin                                                          | License / target                                                | PA#3 purpose and impact                                                                                                                                                                                                        |
| ------------------------------ | --------------------------------------------------------------------- | --------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Go                             | 1.27.4                                                                | BSD-3-Clause; Linux amd64                                       | Existing WSL toolchain. Separate API, worker and checker commands; standard library HTTP, JSON, structured logs and session-token generation.                                                                                  |
| PostgreSQL driver              | `github.com/jackc/pgx/v5 v5.11.0`                                     | MIT; pure Go                                                    | Durable submissions, queue leases, findings, TA decisions and atomic budget reservations. Commit exact transitive versions and checksums.                                                                                      |
| Password hashing               | `golang.org/x/crypto v0.59.0`, Argon2id                               | BSD-3-Clause; Go >=1.26                                         | Hash passwords for synthetic test accounts. Bounded settings and random salts; opaque server sessions mean no JWT library is needed for this checkpoint.                                                                       |
| Database                       | Existing PostgreSQL 18 image, pinned to its resolved immutable digest | PostgreSQL License; Linux amd64                                 | One store for the PA#3 flow and restart recovery. Local database binds to loopback. Additive initialization only; no destructive migration.                                                                                    |
| C++ compiler / syntax analyzer | Debian LLVM/Clang 19 and libclang C API; GNU C++17 compiler           | LLVM Apache-2.0 with exceptions; GCC GPL with runtime exception | Native files only in isolated execution/checker images. Record exact installed package versions and image digest. A small analyzer resolves actual declarations, including namespace/using/macro traps. Go has no CGO binding. |
| Sandbox                        | Existing Docker 29.5.3                                                | Apache-2.0 client/engine components                             | Qualify all nine controls for compile/run and isolate parsing of untrusted C++. Docker control never enters a student container. No privileged execution or host-directory mounts.                                             |
| Frontend                       | Existing Node >=22.13, locked Next.js/React packages                  | Existing lockfile                                               | Add the required sign-in, authoring, submission and TA review screens without a new frontend runtime dependency.                                                                                                               |
| Provider transport / logging   | Go standard library                                                   | BSD-3-Clause                                                    | Real-model HTTP adapter, strict schema checks, timeouts, per-call request IDs/usage/cost logs; fake provider in all unit/contract tests. No provider SDK or telemetry package required.                                        |

Pins checked against [pgx v5.11.0](https://github.com/jackc/pgx/releases/tag/v5.11.0)
and [Argon2 v0.59.0](https://pkg.go.dev/golang.org/x/crypto@v0.59.0/argon2).
Runtime/image digests and installed package versions must be recorded during the
actual clean build. They are not yet installation or isolation evidence.

The default per-lab cap stays USD 0.25. Provider/model/pricing and hosting must be
selected before a separately authorized paid evaluation or public deployment.
The model-off checker needs neither credentials, Docker socket access nor network:
its image includes the analyzer/compiler and only the staged dataset inputs.
The online worker runs syntax analysis in a separate secret-free container before
calling the provider from the trusted process. Never invoke an unisolated compiler
or parser alongside a live model key.
