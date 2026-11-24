# PA#3 checker image

The dataset-independent marking command from the assignment:

```bash
docker build -f apps/backend/checker/Dockerfile -t leetkob-checker:pa3 apps/backend

docker run --rm --network none --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 64 --memory 512m --memory-swap 512m \
  --user 65534:65534 \
  -v "$PWD/evaluation/team-cases:/data:ro" \
  -v /tmp/checker-out:/out \
  -e MODEL_OFF=1 \
  leetkob-checker:pa3 --data /data --predictions /out/predictions.jsonl
```

Behavior:

- One command discovers problems, statements, constraints and solutions
  generically; there is no per-problem switch. Labels, tags, tests and
  `eval.mjs` are never opened.
- Ban-list constraints (`Do not use`/`Do not call` with a backticked name) are
  decided by the bundled libclang analyzer. Every other constraint is written
  as `unsure`: the offline image never calls a provider, so `MODEL_OFF=1` is a
  statement of fact, not a switch that could be forgotten. Online semantic
  checking runs in the worker, behind the guarded adapter and budget.
- Output is one validated JSONL line per solution/constraint pair, written
  atomically. An unconfigured analyzer never fabricates a violation; it yields
  unsure.
- The image runs as uid 65534 with no network, no Docker socket and no
  credentials. `CHECKER_ANALYZER` points at the prebuilt analyzer, so nothing
  is compiled on a trusted host at run time.

Scoring stays separate: run the unmodified `eval.mjs` from the pinned official
tree against the prediction file. Exit 0 or 1 is a valid run outcome; exit 2
means invalid input. No scorer result is produced or claimed by this image.

## Real-model evaluation mode

The image defaults to `MODEL_OFF=1` (semantic pairs unsure). For the
separately authorized dev-set evaluation, provide `MODEL_OFF=0` plus
`MODEL_URL`, `MODEL_NAME`, `MODEL_TOKEN`, optional
`MODEL_INPUT_MICRO_USD_PER_TOKEN`/`MODEL_OUTPUT_MICRO_USD_PER_TOKEN` and
`MODEL_CALL_LOG`. Every dispatch is preceded by an atomic reservation
against the USD 0.25 lab cap; HTTP 429 responses are retried a bounded
number of times; provenance goes to the call log (request ID, tokens,
elapsed, cost, prompt hash). The image carries CA certificates so the
provider can be reached over TLS, and it never logs or commits the token.
