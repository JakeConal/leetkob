# PA#3 checker image

Stage prediction inputs separately from labels, tests and the scorer, then run
this dataset-independent command from the repository root:

```bash
docker build -f apps/backend/checker/Dockerfile -t leetkob-checker:pa3 apps/backend
checker_inputs=$(mktemp -u /tmp/leetkob-checker-inputs.XXXXXX)
node scripts/evaluation/sources.mjs stage evaluation/official "$checker_inputs"
checker_out=$(mktemp -d /tmp/leetkob-checker.XXXXXX)
chmod 0777 "$checker_out"  # prediction artifacts only; uid 65534 writes here

docker run --rm --network none --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 64 --memory 512m --memory-swap 512m \
  --user 65534:65534 \
  -v "$checker_inputs:/data:ro" -v "$checker_out:/out" \
  -e MODEL_OFF=1 \
  leetkob-checker:pa3 --data /data --predictions /out/predictions.jsonl
```

The same staging command accepts `evaluation/team-cases` or a new dataset in the
supported layout. Discovery never switches on a known problem ID. The image
runs libclang inside its credential-free container, decides ban-list constraints
and emits `unsure` for other constraints. `MODEL_OFF=1` is selected before any
provider configuration or credential access. Output is atomic JSONL, one row
per solution/constraint pair. No score is claimed by prediction generation.

Run scoring separately in the trusted context:

```bash
node evaluation/official/eval.mjs --data evaluation/official \
  --predictions "$checker_out/predictions.jsonl" --json "$checker_out/summary.json"
node scripts/evaluation/sources.mjs verify evaluation/official
```

Scorer exit 0 passes thresholds; exit 1 is a valid run below a threshold; exit 2
means invalid input. Keep the official scorer and all 128 input files unchanged.
The model-off 102-pair check decides all 55 ban-list pairs and leaves all 47
semantic pairs `unsure`, with zero wrongful deductions and zero provider calls.

## Real-model mode

The bundled offline image has a native parser and must never receive a provider
key. Live evaluation instead runs the trusted Go checker with access to Docker
and the separately qualified `leetkob-local-analyzer:pa3` image.
`CHECKER_ISOLATED_ANALYZER=1` is mandatory before provider setup; parsing executes
in a credential-free container through the isolation controller.

Live configuration requires `MODEL_OFF=0`, HTTPS `MODEL_URL`, `MODEL_NAME`, a
private `MODEL_TOKEN`, explicit nonnegative
`MODEL_INPUT_MICRO_USD_PER_TOKEN`/`MODEL_OUTPUT_MICRO_USD_PER_TOKEN`, and optionally
`MODEL_CALL_LOG`. Do not type a key into chat, a command argument or a committed
script. Every dispatch reserves against the USD 0.25 lab cap. A schema-repair
attempt reserves again; transport errors have no hidden retry. Unknown usage
has no fabricated cost. Logs contain request ID, tokens, elapsed time and prompt
hash, with no key or prompt body.

The production worker uses a private token **file**, persistent PostgreSQL
reservations and additional per-lab/daily request ceilings. Its operator setup
is documented in [`deploy/NIM.md`](../../../deploy/NIM.md). The historical first
real evaluation is preserved in [`evaluation/dev-run/`](../../../evaluation/dev-run/README.md),
including its original 78.4% recall and the safety warning about the old command.
No unit or contract test makes a real model request.
