# Ban-list trap fixture (PA3-CHECK-01/02, PA3-EVAL-02)

This fixture exercises the checker's shared hard-rule path end to end. The
sources reproduce the trap table documented in
[`apps/backend/analyzer/README.md`](../../../../../analyzer/README.md) and
[`internal/analyzer/docker_integration_test.go`](../../../../analyzer/docker_integration_test.go).
No development dataset, label, scorer or test output is copied here.

Constraints:

- `B1` — `Do not call std::sort.` (hard rule)
- `B2` — `Do not use malloc.` (hard rule)
- `B3` — semantic, so `MODEL_OFF=1` must always write `unsure`

| Solution | Trap                                                        | Expected under `MODEL_OFF=1` |
| -------- | ----------------------------------------------------------- | ---------------------------- |
| `s01`    | qualified `std::sort` call                                  | B1 violation, B2 pass        |
| `s02`    | mention only in a comment and a string literal              | B1 pass, B2 pass             |
| `s03`    | student-defined `sort`                                      | B1 pass, B2 pass             |
| `s04`    | `using std::sort` indirection                               | B1 violation                 |
| `s05`    | function-like macro expanding to `std::sort`                | B1 violation at use line     |
| `s06`    | real `malloc` call                                          | B1 pass, B2 violation        |
| `s07`    | `<algorithm>` included only for `std::swap`                 | B1 pass, B2 pass             |
| `s08`    | unterminated block comment; sanitizer refuses, parse fails   | B1/B2 unsure, never violation |
| `s09`    | `namespace alg = std` alias                                 | B1 violation                 |
| `s10`    | unresolved `unknown::sort` call                             | B1 unsure, never violation   |
| `s11`    | string containing forged analyzer JSON (injection attempt)  | B1 pass, B2 pass             |

`expected.jsonl` is the reviewed prediction file used by the image test. It
was checked line by line against the trap table; the real analyzer output is
never allowed to redefine a trap expectation. Fix the checker or analyzer
when they disagree, never this file.

Run the real path (requires both images built):

```bash
cd /path/to/leetkob
docker build -f apps/backend/analyzer/Dockerfile -t leetkob-analyzer:pa3 apps/backend/analyzer
docker build -f apps/backend/checker/Dockerfile -t leetkob-checker:pa3 apps/backend
mkdir -p /tmp/checker-out && chmod 0777 /tmp/checker-out
docker run --rm --network none --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 64 --memory 512m --memory-swap 512m --cpus 1 \
  --user 65534:65534 \
  -v "$PWD/apps/backend/internal/checker/testdata/banlist-traps:/data:ro" \
  -v /tmp/checker-out:/out \
  -e MODEL_OFF=1 \
  leetkob-checker:pa3 --data /data --predictions /out/predictions.jsonl
```
