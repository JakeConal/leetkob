# LeeTKOB fixed C++17 execution image

This directory owns the PA#3 sandbox boundary for untrusted C++ submissions:

- `Dockerfile` — the fixed Linux image (compiler, C++ runtime, small run
  harness, no services).
- `run-harness` — the only entry point used for execution; compilation runs
  `g++` directly, and the image intentionally has no `ENTRYPOINT`.
- `attacks/` — attack submissions exercised by the Go controller tests in
  [`internal/sandbox`](../../internal/sandbox). These fixtures exist before
  any execution path is enabled.

See design decision 5 ("Qualify execution and parsing isolation before
enabling source execution") and decision 6 (compilation must not see
secrets) in
[`openspec/changes/implement-pa3-go-prototype/design.md`](../../../../openspec/changes/implement-pa3-go-prototype/design.md).

## Build

From `apps/backend`:

```bash
docker build --tag leetkob-sandbox:pa3 sandbox
```

The build pins the base image by resolved digest and the `g++` package
version. The resolved versions below were recorded from a clean build on
2026-11-23 (Linux amd64, Docker 29.5.3). Repositories keep the current
stable package available, so a moved pin fails the build instead of
silently changing the toolchain.

Base-image provenance was captured after pulling the tag:

```bash
docker pull debian:trixie-slim
docker inspect --format '{{index .RepoDigests 0}}' debian:trixie-slim
# debian@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2
```

| Component | Pin |
| --- | --- |
| Base image | `debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2` (Debian 13, slim) |
| Build output | local tag `leetkob-sandbox:pa3`; publish and pin the registry digest before deployment |
| Image size | `129607560` bytes (≈124 MiB uncompressed) |
| `g++` package | `4:14.2.0-1` |
| `g++` compiler version | Debian 14.2.0-19 |
| `g++-14` / `gcc-14` / `cpp-14` toolchain | `14.2.0-19` |
| `libstdc++6` | `14.2.0-19` |
| `libstdc++-14-dev` | `14.2.0-19` |
| `binutils` | `2.44-3` |
| `libc6` | `2.41-12+deb13u5` |

The full installed-package inventory is reproducible with:

```bash
docker run --rm leetkob-sandbox:pa3 \
  dpkg-query -W -f='${Package}=${Version}\n' | sort
```

`debian:trixie-slim` also carries `debian@sha256:a72b…` as its
`RepoDigest`; that is the immutable base pin used by the `FROM` line. A
locally built image has no registry `RepoDigest`, and BuildKit attestation
annotations make repeated local builds produce different manifest-list
digests even when the filesystem and package versions match. The
reproducibility evidence is therefore the pinned base digest plus the
resolved package inventory above, not a local build digest. When the image
is pushed, record and deploy its registry digest instead of the mutable
`:pa3` tag.

The image also carries OCI labels for the base digest and `g++`
package/version; `TestImageMetadataIsUnprivilegedAndSecretFree` asserts
them, so a silent base or toolchain change fails the suite even if the tag
still resolves.

The image contains the base Debian slim packages plus the compiler and C++
runtime (`g++` and `libstdc++6`; no other package is installed by the
build), `/bin/sh` (used by the harness) and the harness itself. It has no
SSH/service, package-manager index, Docker client, credentials, environment
secrets or network configuration.

## Run harness

`/usr/local/bin/run-harness` is a short POSIX shell script that disables
core dumps, changes to `/work` and `exec`s `/work/program`. The image has
no `ENTRYPOINT`, so the controller always passes the exact command
(`g++ …` or the harness path) and the argument vector is inspectable.

## Controller contract

`internal/sandbox.Controller.Judge` compiles one submission and then runs
each test case against the same artifact. Every phase uses the same fixed
image and the same boundary; nothing is compiled or parsed on the trusted
host.

| Control | Enforced by |
| --- | --- |
| No network | `--network none` for compile and run |
| Non-root | `--user 65534:65534`, and `USER 65534:65534` in the image |
| Read-only root | `--read-only`, with a private `/tmp` tmpfs |
| No host binds | exactly one `--mount` of a freshly copied disposable work directory (`/work`); read-only during execution |
| No Docker socket | the socket is never mounted and does not exist in the image; `host_probe.cpp` proves it is absent |
| No capabilities / escalation | `--cap-drop ALL`, `--security-opt no-new-privileges:true` |
| Memory including swap | `--memory N --memory-swap N` (equal means no swap) |
| PIDs / threads | `--pids-limit 64` |
| CPU | `--cpus 1` quota |
| Output | at most 1 MiB of stdout and 1 MiB of stderr are stored; the rest is drained and the run is RE |
| Wall deadline | host-side context; on expiry the whole container is removed with `docker kill` then `docker rm --force` |
| Compile wall | fixed 10 s host-side deadline |
| Source cap | 64 KiB, rejected before any container starts |
| Secrets | no host environment is forwarded; the only mount is the work copy; the image has no service secrets |

Compile and run limits differ only where the compiler needs more room:
compile uses 512 MiB and 1 CPU (both still bounded); execution uses the
lab's memory ceiling, 64 PIDs and 1 CPU. The per-test wall is
`3 × cpuSeconds` (default 3 s when `cpuSeconds` is unset).

Docker itself injects read-only file mounts for `/etc/hosts`,
`/etc/hostname` and `/etc/resolv.conf` into every container, including
with `--network none`. They contain only container metadata/name-server
information, never host credentials, and `host_probe.cpp` verifies that no
arbitrary host path or Docker socket is reachable. Removing those managed
files is not expressible with the Docker runtime flags; this is recorded
as a known boundary detail rather than hidden.

`Judge` never compiles on the trusted host and returns either:

- a `JudgeResult` with `CE`, `TLE`, `MLE` or `RE` per the mapping below, or
- a distinct `*sandbox.InfrastructureError` when Docker, the image, the
  container start or inspection failed.

Infrastructure failures carry no verdict; they must be retried/reported as
an operational state, never stored as a student failure.

### Verdict mapping

| Observation | Verdict |
| --- | --- |
| compile exit ≠ 0 or compile wall expired | `CE` (compile timeout sets `TimedOut`; diagnostics are bounded) |
| Docker reports `OOMKilled` | `MLE` |
| host wall deadline expired | `TLE` |
| non-zero exit, crash, or stdout/stderr overflow | `RE` |
| exit 0, no overflow | `RAN` (token comparison decides AC/WA in task 3.5) |

Memory is reported as `MLE` only when Docker's observed `OOMKilled` state
proves it. If that signal is unavailable (for example a daemon that cannot
inspect the stopped container), the run is `RE` with the raw exit code;
the worker must not claim an unproven MLE. This is the documented
"memory RE otherwise" fallback.

Docker provides a CPU quota (`--cpus`) but no independent per-container
CPU-time kill signal. The quota bounds host consumption, and the
host-enforced wall deadline (three times the lab's CPU-second limit) is the
kill. `TLE` therefore covers both a CPU-bound infinite loop and a sleeping
process. The CPU loop fixture reaches the deadline while the quota keeps it
from monopolising a core.

The 64 KiB source cap is rejected before any container starts as
`ErrSourceTooLarge`, a bounded-admission error. It is neither an
`InfrastructureError` nor a verdict; the submission-ingress layer (task
2.2+) owns the user-facing rejection.

## Attack fixtures

Each fixture prints machine-readable markers so the test asserts the
observed containment rather than only the configured flags.

| Fixture | Attack | Expected containment |
| --- | --- | --- |
| `attacks/network_probe.cpp` | DNS lookup and TCP connect to `1.1.1.1:53` | `DNS=denied`, `CONNECT=denied`; exits 0 |
| `attacks/cpu_loop.cpp` | infinite CPU loop | host wall deadline, `TLE`, container gone |
| `attacks/wall_sleep.cpp` | `sleep(60)` | host wall deadline, `TLE`, container gone, `SLEPT` never printed |
| `attacks/memory_overflow.cpp` | 8 MiB chunks with page write and read-back | `OOMKilled`, `MLE`, no `ALLOCATION_COMPLETED` |
| `attacks/process_limit.cpp` | thread creation then `fork` until `EAGAIN` | counts stay below the 64 PID limit; a later run is unaffected |
| `attacks/output_flood.cpp` | 16 MiB of stdout | collected output is exactly the 1 MiB cap, `OutputTruncated`, `RE` |
| `attacks/root_probe.cpp` | `geteuid`, `setuid(0)`, capabilities, `/etc/shadow`, rootfs write, `unshare` | `UID=65534`, `CAPEFF=000…`, `NONEWPRIVS=1`, all privileged operations denied |
| `attacks/host_probe.cpp` | Docker socket, host canary path and `/proc/self/mountinfo` | socket and canary absent; only `/work` is a bind mount |
| `attacks/secret_probe.cpp` | environment, canary env/file and compile-time `#include` of a canary path, plus candidate service paths | `ENV_CANARY=absent`, `FILE_CANARY=absent`, `INCLUDE_CANARY=absent`, no canary value in compile or run output |
| `attacks/compile_hang.cpp` | preprocessor blocks reading `/dev/ptmx` forever | compile wall, `CE` with `TimedOut=true` |
| `attacks/compile_error.cpp` | invalid C++17 | `CE` with bounded diagnostics |
| `attacks/compliant.cpp` | reads two integers, prints the sum | compiles and returns token data |

The secret fixtures use synthetic canaries only. The test process places a
canary in its own environment and in a host file outside the work copy,
then asserts that neither is visible to the container. No live credential
is ever used as a probe.

## Tests

```bash
cd apps/backend
go test ./internal/sandbox/
go test -race ./internal/sandbox/
```

The suite:

- builds the image on first use when Docker is available (network is
  needed for that build only);
- skips with a clear message only when the Docker CLI or daemon is
  unavailable;
- fails, rather than skips, if the image cannot be built;
- runs every attack against the real controller and asserts the observed
  container state, including that the container is gone after a wall or
  output case;
- includes `TestNegativeControlDetectsMissingNonRootEnforcement`, which
  runs the same `root_probe.cpp` through a deliberately unconfined,
  test-only helper (`--user 0:0`) and asserts that it reports `UID=0`.
  This proves the assertion can detect absent enforcement; the real
  `Config` has no switch that disables a control.

`TEST_SANDBOX_IMAGE=<digest reference>` runs the suite against a specific
image instead of building `leetkob-sandbox:pa3`.

Focused commands used for this slice's evidence (from `apps/backend`):

```bash
go test -run TestCompliantCpp17CompilesAndReturnsTokenData -v ./internal/sandbox/
go test -run TestOutboundNetworkIsDenied -v ./internal/sandbox/
go test -run TestCPUInfiniteLoopIsTerminated -v ./internal/sandbox/
go test -run TestWallClockSleepIsTerminatedAndContainerRemoved -v ./internal/sandbox/
go test -run TestMemoryAllocationIsBoundedWithReadBack -v ./internal/sandbox/
go test -run TestProcessAndThreadCeilingIsBounded -v ./internal/sandbox/
go test -run TestOutputFloodIsBoundedAndMappedToRE -v ./internal/sandbox/
go test -run TestRootAndPrivilegeProbeIsUnprivileged -v ./internal/sandbox/
go test -run TestNoHostMountsOrDockerSocket -v ./internal/sandbox/
go test -run TestNoSecretVisibleThroughEnvironmentFilesOrInclude -v ./internal/sandbox/
go test -run TestCompileFailureMapsToCE -v ./internal/sandbox/
go test -run TestCompileTimeoutMapsToCE -v ./internal/sandbox/
go test -run TestNegativeControlDetectsMissingNonRootEnforcement -v ./internal/sandbox/
```

## What is proven here, and what is deferred

Proven by this slice (task 3.1): the image is pinned and reproducible, the
controller compiles and runs C++17 only inside the boundary, all nine
rubric minimums have at least one real attack fixture with an observed
containment assertion, compile isolation is covered (including a compile
timeout and a compile-time secret-include probe), source/output/time limits
are enforced, and the negative control shows the assertions are not
vacuous.

Deferred to later tasks:

- **3.2/3.3** — durable queue/worker integration, retry/fencing and the
  actual `pa3` tag CI job that runs this suite on the tag. The controller
  is deliberately independent of the queue so it can be wired without
  changing the boundary. The current implementation requires a local Docker
  daemon (or one that shares the controller filesystem), because the
  disposable work directory is bind-mounted by path; a remote
  `DOCKER_HOST` would need a copy-in mechanism.
- **3.4** — broader analyzer/format coverage and any additional
  platform-specific memory evidence beyond Docker's `OOMKilled`.
- **3.5** — AC/WA token comparison, per-test aggregate scoring and
  student-visible results. This package returns stdout tokens and timing
  only.
