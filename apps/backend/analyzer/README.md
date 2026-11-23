# Isolated libclang analyzer (PA#3 hard rules)

`leetkob-analyzer` resolves the backticked names in ban-list constraints
against the libclang C++17 AST. It is the hard-rule side of PA3-CHECK-01
and PA3-CHECK-02: comments, string literals and harmless includes never
trigger, student-defined names stay student-defined, and namespace
aliases, `using` declarations, macros and wrappers resolve to the actual
declaration. The Go backend invokes this image without CGO and turns any
tooling failure into `unsure`.

The analyzer is also the artifact the offline checker image embeds for
`MODEL_OFF=1`; it needs no Docker socket, provider key, labels or network
inside the checker container.

## Wire protocol

One bounded JSON request on stdin:

```json
{"source": "int main(){return 0;}", "bannedNames": ["std::sort", "sort"]}
```

One bounded JSON reply on stdout, one result per requested name in
request order:

```json
{
  "version": 1,
  "results": [
    {
      "name": "std::sort",
      "verdict": "violation",
      "evidence": [{ "line": 5, "text": "  std::sort(v.begin(), v.end());" }]
    },
    { "name": "sort", "verdict": "pass", "evidence": [] }
  ]
}
```

`verdict` is `violation`, `pass` or `unsure`:

- **violation**: libclang resolved a reference to a declaration whose
  qualified name matches the ban. Evidence is at least one original
  submitted-source line. Passing a ban does not happen by text search.
- **pass**: analysis completed with no declared syntax errors and no
  matching resolved reference.
- **unsure**: the translation unit could not be parsed, an unresolved
  reference could name the ban, or a match had no usable original-line
  evidence. `unsure` never becomes a deduction and is never reported as
  a violation.

Resolution rules (generic; no development-problem answers are built in):

| Case                                                        | Result                                                                                                                |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| Qualified ban `std::sort`, call `std::sort(...)`            | violation at the call line                                                                                            |
| Unqualified ban `sort`, call `std::sort(...)`               | violation at the call line                                                                                            |
| Student defines `namespace sort` or their own `sort`        | pass; declarations defined in the submitted file are treated as student-defined                                      |
| Mention only in a comment or string/raw string              | pass; no AST reference exists                                                                                         |
| `<algorithm>` included only for `std::swap`                 | pass; includes are not scanned as text                                                                                |
| `namespace alg = std; alg::sort(...)`                       | violation; libclang resolves the alias                                                                                |
| `using std::sort; sort(...)` / `using namespace std`        | violation; the call resolves to `std::sort`                                                                           |
| Function-like or object-like macro expanding to `std::sort` | violation at the macro-expansion line, not the `#define` line                                                         |
| Wrapper function containing `std::sort`                     | violation at the `std::sort` reference inside the wrapper                                                             |
| Unexpanded `#define SORT std::sort`                         | pass; a macro definition is not a use                                                                                 |
| Disabled `#if 0` branch                                     | pass; it is not in the parsed AST                                                                                     |
| Unresolved `unknown::sort(...)` / missing include           | unsure (an error diagnostic is not a pass)                                                                            |

Evidence text is read from the submitted source, never from headers.
Multiple references are deduplicated and bounded to four lines per name
and 4,096 bytes per line. If the complete reply would exceed 2 MiB, the
tool writes an all-`unsure` reply instead of an unbounded or fabricated
one.

The Go boundary additionally rejects replies with unknown fields, a
missing/duplicate/unknown name, an invalid verdict, a violation without
evidence, evidence outside the source, or evidence text that does not
match the original line. It returns an error and zero results in those
cases; the caller records `unsure`.

## Limits and isolation

| Control      | Value                                                                                  |
| ------------ | -------------------------------------------------------------------------------------- |
| stdin        | 256 KiB encoded request; source 65,536 bytes; 64 names; 256 bytes per name             |
| stdout       | 2 MiB bounded collection; bounded tool reply                                           |
| time         | 20 s default Go deadline; `--stop-timeout 1`; timed-out container is force-removed      |
| network      | `docker run --network none`                                                             |
| filesystem   | `--read-only` plus `noexec,nosuid` 16 MiB `/tmp` tmpfs; no host mounts                  |
| privileges   | UID/GID 65534, `--cap-drop ALL`, `no-new-privileges`                                   |
| processes    | `--pids-limit 64`                                                                      |
| memory/swap  | 512 MiB memory and equal swap ceiling                                                  |
| CPU          | 1 CPU                                                                                  |
| secrets      | no provider key, dataset label or Docker socket is in the image or its environment      |

## Image pin

Built and recorded on 2026-11-23 from:

| Component             | Pin / version                                                                       |
| --------------------- | ----------------------------------------------------------------------------------- |
| Base image            | `debian@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2` (trixie-slim, Debian 13.7) |
| Local image tag       | `leetkob-analyzer:pa3`                                                               |
| Local image ID        | `sha256:8aed1ee3889b67054b735832174a73031e24d413e3e684914da6d6b9143b0546`             |
| Config digest         | `sha256:f06d57f9e60ac3fb35a789a72532370934ba4598e1fda909e80047c927431640`             |
| Image size            | 276,936,006 bytes (264 MiB)                                                          |
| Linux / arch          | linux/amd64                                                                          |
| clang / libclang      | `clang-19 1:19.1.7-3+b1`, `libclang-19-dev 1:19.1.7-3+b1`, `libclang1-19 1:19.1.7-3+b1` |
| GNU C++ compiler      | `g++-14 14.2.0-19`                                                                   |
| libstdc++ dev headers | `libstdc++-14-dev 14.2.0-19`                                                         |
| C library             | `libc6 2.41-12+deb13u5`                                                              |
| Runtime user          | `uid=65534(nobody) gid=65534(nogroup)`                                               |

The Debian base digest is the reproducible pin; the package versions are
the installed `clang-19`, `libclang-19-dev` and `libstdc++-14-dev`
packages. Rebuild with the same tag; CI records the resolved digests for
the release artifact.

## Build and run

```bash
docker build -t leetkob-analyzer:pa3 apps/backend/analyzer

printf '%s' '{"source":"#include <algorithm>\nint main(){return 0;}\n","bannedNames":["std::sort"]}' \
  | docker run --rm -i --network none leetkob-analyzer:pa3
```

The Go wrapper is the only supported caller:

```bash
cd apps/backend
ANALYZER_IMAGE=leetkob-analyzer:pa3 go test ./internal/analyzer/
```

The container tests skip **only** when Docker (CLI or daemon) is
unavailable. When Docker is available, the image must already be built;
the test fails with the build command rather than silently skipping the
trap suite. Image builds may use the network for `apt`; the tests do not.

## Tests

- `internal/analyzer/analyzer_test.go` covers request/reply bounds,
  schema/evidence validation, timeout mapping and the pure-Go all-unsure
  fallback without Docker.
- `internal/analyzer/docker_integration_test.go` runs the table of hard-rule
  traps against the built image: qualified/unqualified calls, student
  namespace/function `sort`, comment/string/raw-string-only mentions,
  harmless `<algorithm>` for `std::swap`, namespace alias, using
  declaration/directive, function/object macros (with expansion-line
  evidence), wrapper function, unexpanded macro, `#if 0`, and unresolved
  input becoming unsure.

## Deferred wiring

This task builds the analyzer and the Go boundary only. Sanitization and
identity stripping (task 5.2), the checker's MODEL_OFF/model-dispatch
wiring (task 7.2) and the checker image composition (task 7.1) are owned
by other slices. The offline checker image includes this binary locally;
it must not require nested Docker.
