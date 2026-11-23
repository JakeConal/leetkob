# PA#3 team cases

Six-output team-authored evaluation cases for the constraint checker. Each
case is a small C++17 problem in the same layout the checker discovers
(`problems/<id>/{statement.md,constraints.json,solutions/*.cpp}`), so the
marking command runs on this folder unchanged with no per-problem switches.

The labels in `labels/labels.jsonl` are established independently of the
checker's predictions: each verdict was decided by reading the solution and
the constraint text, before running any tool. The labels only cover
ban-list constraints, whose true outcome is deterministic; semantic
constraints are intentionally absent so no model-dependent expectation is
invented.

Tags and intent:

| Case | Tag | Trap / attack |
| --- | --- | --- |
| `sum-array` | trap | the failing solution still produces the correct output, so only the rule can catch it |
| `sort-chars` | trap | a student-defined function named `sort` must not match a ban on `std::sort` |
| `count-evens` | injection | prompt-injection text in comments and a string literal, expecting zero effect |
| `median` | indirection | a macro expands to the banned call; evidence must cite the expansion line |
| `upper-line` | indirection | `malloc` reached through a normal allocation path |

Every solution is required to be Accepted: `scripts/evaluation/team-cases.test.mjs`
compiles each one with `g++ -std=c++17 -O2` and runs the case's tests,
comparing tokens. The same test asserts the labels cover every
solution/constraint pair exactly once with a valid verdict.

Provenance: authored by the team for this repository; no official dataset
bytes are copied here. The official dev set remains fetched-only under
`evaluation/official-source.json` because its pinned tree carries no
redistribution license.
