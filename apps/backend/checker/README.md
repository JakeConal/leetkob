# Checker image (PA#3 task 7.1)

The marker's dataset-independent command:

```bash
checker --data <dataset-dir> --predictions <output.jsonl>
```

One JSON line per solution/constraint pair, matching
[`docs/contract/schemas/checker-prediction.v1.json`](../../../docs/contract/schemas/checker-prediction.v1.json).
Discovery uses only documented layout names: `statement*`, `constraints*`
(JSON or one-per-line), and `solutions/<id>.{cpp,cc,cxx}` either inside a
problem directory or under the dataset-level `solutions/<problem>/` directory.
`problems/`, `labs/`, and direct problem directories are all supported. No
problem ID, prompt, switch, table, label or scorer read is compiled in.

## Verdict status in this slice

Task 7.1 delivers **discovery and output validity**. The hard-rule analyzer
(task 5.1) and semantic model pipeline (tasks 5.3, 5.4 and 7.2) are not wired
yet, so this image writes `unsure` with empty evidence for every pair. That is
honest output: it neither fabricates a violation nor claims a pass. Task 7.2
replaces the placeholder verdict branch with hard-rule and model verdicts.
Ban-list names are already parsed generically from `Do not use` / `Do not call`
texts so the analyzer can reuse them.

`MODEL_OFF=1` is selected before any provider could exist. This image contains
no model client and opens no network in any mode. It needs no keys and no
Docker socket, and is verified with `--network none`.

## Build

The build stage is pinned by resolved digest:

```text
golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea
```

Resolved 2026-11-23 with `docker buildx imagetools inspect golang:1.27.4-bookworm`
(equivalent: `docker pull` then `docker image inspect`). Re-resolve and update the
Dockerfile before rebuilding on a later date. The runtime stage is `scratch`: a
statically linked binary with no shell, package manager, CA bundle or network
tooling, running as UID/GID 65532.

```bash
cd /path/to/leetkob
docker build -f apps/backend/checker/Dockerfile -t leetkob-checker:7.1 apps/backend
```

The build context is `apps/backend`. The image build fetches only the pinned Go
base image; `go build` compiles `cmd/checker`, whose only imports are the Go
standard library and `internal/checker`, so the module proxy is never needed.

## Run (marker path)

```bash
mkdir -p /tmp/checker-out && chmod 0777 /tmp/checker-out

docker run --rm --network none --read-only \
  --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 64 --memory 256m \
  -v /path/to/prediction-inputs:/data:ro \
  -v /tmp/checker-out:/out \
  -e MODEL_OFF=1 \
  leetkob-checker:7.1 --data /data --predictions /out/predictions.jsonl
```

`/out` must be writable by UID 65532 (for example
`mkdir -p /tmp/checker-out && chmod 0777 /tmp/checker-out`, or
`--user "$(id -u):$(id -g)"`). The dataset mount is read-only.

The image exits `0` after writing valid JSONL, `2` for an invalid invocation or
an invalid dataset (for example a problem without constraints or a source over
the documented 65,536-byte limit). Run the unmodified scorer separately; do not
mount labels, the scorer or tests into this container.

## Tests

```bash
cd apps/backend
go test ./internal/checker/ ./cmd/checker/
go test -race ./internal/checker/ ./cmd/checker/
```

`internal/checker/testdata/unfamiliar` is the PA3-EVAL-01 fixture: invented
problem IDs in four layouts, invalid `labels.json`, invalid `eval.mjs`, and test
data that must never be opened. Tests also run the loader with a recording
filesystem, compare outputs with and without metadata, and verify deterministic
bytes across runs.
