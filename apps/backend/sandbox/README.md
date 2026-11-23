# C++17 execution sandbox

The controller in `apps/backend/internal/sandbox` builds the image in
`cpp17/` and runs untrusted compilation and execution inside it. Docker is
invoked only by that controller. Student containers are not privileged, do
not receive the process environment, and do not get a host bind or the
Docker socket.

Task 3.5 maps the controller's raw signals to student verdicts. Until that
lands, the signals are:

| Observation                                                                      | Later verdict                                 |
| -------------------------------------------------------------------------------- | --------------------------------------------- |
| Compiler non-zero exit, or the compilation wall deadline                         | CE                                            |
| Wall deadline, or a CPU-time death that is not `OOMKilled` and not an output cap | TLE                                           |
| `State.OOMKilled`                                                                | MLE                                           |
| Output cap, or any other non-zero exit                                           | RE                                            |
| Returned Go error (daemon, copy, rejected invocation)                            | infrastructure failure, not a student verdict |

Docker sets `State.OOMKilled` for the cgroup memory killer, which is distinct
from the SIGKILL this controller sends when the wall deadline fires. A backend
that cannot report that bit has to document memory deaths as RE before it is
used. The memory attack prints `memory.max` and `memory.swap.max`, then reads
allocated pages back so a compiler cannot delete the allocation. Containment
means `readback_ok` is absent. On cgroup v2, Docker reports a disabled swap
account as `MemorySwap` 0 and `memory.swap.max` 0; that is a ceiling of no
swap, which is the equal-ceiling outcome. A reported swap limit above the
memory limit, or an unlimited swap value, is a failure.

Docker still injects its own `/etc/hosts`, `/etc/hostname`, and
`/etc/resolv.conf` mounts. The controller does not add a bind. The mount
attack checks a host-only synthetic canary and the Docker socket, and the
secret attack checks environment, files, and preprocessor inclusion. Canaries
are synthetic strings created by the test. A live credential is not a probe.

## Future CI job

Tag CI (task 8.1) is not wired here. The job must run this from a Linux host
whose Docker daemon can build and start containers, with no model credential
in the environment and without changing workflow permissions in this change:

```bash
cd apps/backend
CGO_ENABLED=0 go test ./internal/sandbox/... -count=1 -timeout 20m
```

`go test` builds the image and runs one attack fixture per control. If the
`docker` binary or the daemon is missing, the live tests skip with
`docker not available` or `docker daemon not available`. A skip is not a pass.
Unit tests still require the create argv below, so removing a control fails
the suite even when Docker is absent.

The controller's build is exactly:

```text
docker build -t leetkob-cpp17:pa3 apps/backend/sandbox/cpp17
```

A one-second, 256 MiB judging run is exactly this `docker create` (the
container name in production is `leetkob-sbx-` plus 16 random hex digits).
There is no `--privileged`, `--env-file`, `-e`, `-v`, `--volume`, or `--mount`:

```text
docker create --name leetkob-sbx-0123456789abcdef --network none --user 10001:10001 --workdir /work --log-driver json-file --log-opt max-size=1m --log-opt max-file=1 --cap-drop ALL --security-opt no-new-privileges:true --pids-limit 16 --memory 268435456 --memory-swap 268435456 --ulimit cpu=1:1 leetkob-cpp17:pa3 /work/prog
```

Isolated compilation is the same shape with the compilation ceiling (10 CPU
seconds, 512 MiB memory and swap, 16 processes) and the fixed compiler
arguments:

```text
docker create --name leetkob-sbx-0123456789abcdef --network none --user 10001:10001 --workdir /work --log-driver json-file --log-opt max-size=1m --log-opt max-file=1 --cap-drop ALL --security-opt no-new-privileges:true --pids-limit 16 --memory 536870912 --memory-swap 536870912 --ulimit cpu=10:10 leetkob-cpp17:pa3 g++ -std=c++17 -O2 -pthread -o /work/prog /work/main.cpp
```

The wall deadline is not a Docker flag. When it fires, the controller runs
`docker kill` on that container and then `docker rm -f`. Output past the
collection cap is discarded and the same kill runs. Source and compiler
output cross the boundary only through `docker cp` into the container
filesystem. Nothing from the host workspace is mounted, and the copied binary
is never executed on the host.

Attack fixtures live in `attacks/`: `network.cpp`, `cpu.cpp`, `wall.cpp`,
`memory.cpp`, `processes.cpp`, `output.cpp`, `nonroot.cpp`, `mounts.cpp`,
and `secrets.cpp`.
