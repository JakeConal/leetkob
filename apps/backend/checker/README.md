# Checker image (PA#3 tasks 7.1-7.2)

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

## Verdict selection

| Constraint                   | Decided by                                                 | Never                                   |
| ---------------------------- | ---------------------------------------------------------- | --------------------------------------- |
| `Do not use` / `Do not call` | bundled libclang analyzer (syntax + resolved declarations) | model, text search, labels, test output |
| everything else (semantic)   | guarded model port behind a priced budget reservation      | hard rules, labels, deductions          |

Hard-rule decisions are final for the pair: comments, string/raw-string
literals, `<algorithm>` included only for `std::swap`, student-defined `sort`
and unexpanded macros do not become violations. Analyzer failure, protocol or
evidence mismatch, an unresolved reference and an unparseable translation unit
become `unsure`, never a violation. A ban-list text without backticked names is
also `unsure` because nothing can be resolved generically.

Semantic constraints are batched per solution. Before dispatch the core:

1. selects `MODEL_OFF` (offline runs stop here),
2. strips comments and known identities with `internal/sanitize` while keeping
   original line numbers (unsafe input becomes `unsure` with no transmission),
3. refuses if no semantic port is configured,
4. reserves a conservative maximum against the lab cap (unknown price refuses
   dispatch), then
5. validates pass/violation/unsure and original-line evidence. Missing results,
   timeouts, provider/schema/evidence failures become distinct `unsure` causes.

`MODEL_OFF=1` therefore writes `unsure` for every semantic pair, decides every
ban-list pair it can resolve, and reports `"modelCalls":0`. No provider client,
credential read, budget reservation or network call happens on that path.

## Build

The build context is `apps/backend`. The final stage reuses the qualified
analyzer image, so its pinned Debian + Clang runtime is also the checker
runtime and `/usr/local/bin/leetkob-analyzer` is bundled. No Docker socket is
present in the image.

```bash
cd /path/to/leetkob
docker build -f apps/backend/analyzer/Dockerfile -t leetkob-analyzer:pa3 apps/backend/analyzer
docker build -f apps/backend/checker/Dockerfile -t leetkob-checker:pa3 apps/backend
```

The Go build stage is pinned by resolved digest:

```text
golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea
```

Resolved 2026-11-23 with `docker buildx imagetools inspect golang:1.27.4-bookworm`.
The analyzer image pins its own base and records package versions in
[`apps/backend/analyzer/README.md`](../../analyzer/README.md). Re-resolve tags
and digests before rebuilding on a later date.

## Run (marker path)

```bash
mkdir -p /tmp/checker-out && chmod 0777 /tmp/checker-out

docker run --rm --network none --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 64 --memory 512m --memory-swap 512m --cpus 1 \
  --user 65534:65534 \
  -v /path/to/prediction-inputs:/data:ro \
  -v /tmp/checker-out:/out \
  -e MODEL_OFF=1 \
  leetkob-checker:pa3 --data /data --predictions /out/predictions.jsonl
```

`/out` must be writable by UID 65534 (for example
`mkdir -p /tmp/checker-out && chmod 0777 /tmp/checker-out`). The dataset mount
is read-only; the container has no keys, no Docker socket and no network. Do
not mount labels, the scorer or tests into it. The command applies its own
ten-minute deadline.

The image exits `0` after writing valid JSONL, `2` for an invalid invocation or
an invalid dataset (for example a problem without constraints or a source over
the documented 65,536-byte limit). It never runs a scorer.

## Scorer interaction

The checker never invokes, imports, copies or parses `eval.mjs`. The marker
runs the unmodified scorer separately after this command, exactly as
`evaluation/README.md` documents:

```bash
node /path/to/official/eval.mjs \
  --data /path/to/official \
  --predictions /tmp/checker-out/predictions.jsonl \
  --json /tmp/summary.json
```

For PA#3, scorer exit `0` or `1` is a valid run; exit `2` means invalid
predictions or inputs and is a failed run. This repository does not vendor the
official scorer and does not claim a score. When a verified local checkout is
available, `TestOptionalUnmodifiedScorerAcceptsPredictions` accepts
`LEETKOB_OFFICIAL_EVAL_DIR` and asserts the real scorer exits `0` or `1`, never
`2`; without it the test skips with that instruction.

## Environment

| Variable                       | Meaning                                                                        |
| ------------------------------ | ------------------------------------------------------------------------------ |
| `MODEL_OFF=1`                  | Offline selection before any provider/budget construction.                     |
| `CHECKER_ANALYZER`             | Bundled analyzer executable; the image sets `/usr/local/bin/leetkob-analyzer`. |
| `MODEL_BASE_URL`, `MODEL_NAME` | Non-secret provider configuration; absent means semantic pairs are unsure.     |
| `MODEL_API_KEY`                | Read lazily only on a real dispatch; never present in the image run.           |
| `MODEL_INPUT_CENTS_PER_1K`     | Verified integer-cent input price; absent/zero refuses dispatch.               |
| `MODEL_OUTPUT_CENTS_PER_1K`    | Verified integer-cent output price; absent/zero refuses dispatch.              |
| `MODEL_LAB_ID`                 | Run-scoped spend bucket name for the command (default `checker`).              |

The command's budget store is run-scoped and in memory; the production worker
supplies the durable store. The USD 0.25 per-lab cap comes from
`internal/budget`.

## Tests

```bash
cd apps/backend
go test ./internal/checker/ ./cmd/checker/
go test -race ./internal/checker/ ./cmd/checker/
```

Coverage added by task 7.2:

- `MODEL_OFF=1` never calls the injected semantic transport and decides
  ban-list pairs from the injected hard-rule report.
- Without an analyzer no hard-rule pass or violation is invented.
- Semantic pairs stay unsure without a budget service, with unknown pricing, at
  the cap, without a model port, and when sanitization is unsafe.
- A paid, priced reservation precedes exactly one semantic call; ban-list
  constraints never appear in the model request.
- The exec analyzer validates the wire reply, maps timeouts, and gives the
  child process a rebuilt environment with no provider key or label.
- The analyzed trap fixture `testdata/banlist-traps` is compared with the
  reviewed `expected.jsonl`: five real uses, zero wrongful hard-rule
  deductions, all semantic pairs unsure.
- `TestCheckerImageModelOffRunsWithoutNetworkOrDockerSocket` runs the built
  image with `--network none`, 512 MiB/64 PID limits, no keys and no socket on
  that fixture. It fails (rather than skips) when Docker is present but the
  image is not built, so an absent image is visible.
- `internal/checker/testdata/unfamiliar` remains the PA3-EVAL-01 fixture:
  invented problem IDs in four layouts, invalid `labels.json`, invalid
  `eval.mjs`, and test data that must never be opened.

The image test does not build either image; build both first with the commands
above. It uses `CHECKER_IMAGE` to select a different tag.
