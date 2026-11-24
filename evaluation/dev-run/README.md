# First real-model dev-set run (PA#3)

Authorized by the owner on 2026-11-24. This directory is the committed
evidence for the first genuine model run on the official dev set; it is
not a rehearsal and no number here is invented.

## Provenance

| Item | Value |
| --- | --- |
| Official data revision | `552c811db2d35a23d641bc4fdffc1ac120be1a28` (6 problems, 47 solutions, 102 labelled pairs) |
| Data integrity | 128/128 pinned files verified byte-identically before staging; only statement/constraint/solution inputs were staged (59 files) |
| Checker | `leetkob-checker` image built from this repository at the commit that adds this file; `MODEL_OFF=0`, analyzer bundled, run non-root with no Docker socket |
| Provider / model | NVIDIA NIM (`https://integrate.api.nvidia.com/v1/chat/completions`), `minimaxai/minimax-m3.1-flash` |
| Call policy | price schedule configured (free developer tier: 0 micro-USD/token), per-lab cap enforced before every dispatch, at most 4 transport retries for HTTP 429 |
| Commit | the commit containing this directory; the image was built from the same tree |

The API key was supplied by the owner in an environment file outside the
repository and is not present in any committed file, log or artifact.

## Command

```bash
docker run --rm --read-only --tmpfs /tmp:rw,noexec,nosuid,size=256m \
  --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 128 --memory 1g --memory-swap 1g --user 65534:65534 \
  --env-file /path/outside/repo/nim.env \
  -v "$STAGED_INPUTS:/data:ro" -v "$OUT:/out" \
  leetkob-checker:pa3 --data /data --predictions /out/predictions.jsonl
```

Scoring uses the **unmodified** official scorer:

```bash
node /path/to/pinned/eval.mjs --data /path/to/pinned --predictions predictions.jsonl --json scorer-summary.json
```

## Result (first run, 2026-11-24)

| Metric | Value |
| --- | ---: |
| Pairs | 102 |
| Wrongful deductions | **0** |
| Caught violations | 29 / 37 |
| Recall | **78.4 %** |
| Unsure pairs | 4 (3.9 %) |
| Scorer exit | **1** (threshold miss — a valid PA#3 run; exit 2 would mean invalid input) |

The scorer reports a threshold failure because recall is below the PA#4
90 % target. PA#3 imposes **no overall eval pass threshold**: the honest
first-run metrics are recorded as-is, and no label, case or threshold was
adjusted. The remaining misses are semantic constraints the model answered
`pass` where the label is `violation`; the two provider errors in the call
log are rate-limited dispatches that exhausted the bounded retries and
became unsure.

## Provider usage and cost (task 7.5 arithmetic)

From `call-log.jsonl` (38 successful dispatches, 2 rate-limited failures):

- 34,810 input tokens and 11,526 output tokens in total; average 916 in /
  303 out per dispatch; average latency 12.2 s.
- One semantic dispatch covers all semantic constraints of one solution.
  For a lab of **80 submissions** that is about 80 dispatches ≈ **73,280
  input + 24,240 output tokens** (≈ 97.5k tokens) and ≈ 16 minutes of
  sequential provider time.
- Cost at the free NVIDIA developer tier is **USD 0.00**. The adapter
  still requires an explicit price schedule: if the deployment moves to a
  paid tier, operators must configure micro-USD-per-token prices, and an
  unknown price refuses dispatch rather than charging zero. The unchanged
  per-lab cap is **USD 0.25**, enforced by atomic reservations before every
  call and retry.

## Files

- `predictions.jsonl` — 102 model/rule predictions for the pinned dev set.
- `scorer-summary.json` — the unmodified scorer's JSON report for those
  predictions.
- `call-log.jsonl` — one line per dispatch: timestamp, model, provider
  request ID, tokens, elapsed time, cost, sanitized-prompt SHA-256 and
  attempt count. Prompts, source code, identities and credentials are not
  logged.

Reproduction note: the marker should see exit **0 or 1** from their
unmodified scorer for this file; exit 2 indicates invalid input and is
never acceptable.
