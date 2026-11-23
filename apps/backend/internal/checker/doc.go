// Package checker is the shared, dataset-independent constraint checker used
// by the offline marking command (cmd/checker) and, later, by the judging
// worker.
//
// # Scope of task 7.1 (issue #38)
//
// This package discovers problems, statements, constraints and solution
// sources from the documented dataset layout and writes one deterministic JSONL
// prediction per solution/constraint pair, matching
// docs/contract/schemas/checker-prediction.v1.json. Discovery is generic: no
// problem-ID table, per-problem switch, prompt, label or scorer read exists.
// Dataset-level metadata such as labels.jsonl, eval.mjs, reports and test data
// is never opened, even when it sits next to the inputs.
//
// # Verdict scope
//
// The hard-rule analyzer (task 5.1) and the semantic model pipeline (tasks 5.3,
// 5.4 and 7.2) are not wired into this core yet. Until task 7.2 connects them,
// BuildPredictions writes an honest "unsure" verdict with empty evidence for
// every pair. It never fabricates a pass or a violation. Ban-list names are
// nevertheless parsed here (BannedNames) so the analyzer can consume them
// without re-implementing the constraint grammar. When MODEL_OFF=1, Args.ModelOff
// is selected before any provider exists; this package imports no network
// package at all, so both online and offline runs are network-free in this
// slice.
//
// # Supported layout
//
// The dataset root may contain problem directories directly, and/or the
// documented "problems" and "labs" container directories:
//
//	<dataset>/<problem>/statement.{md,txt}          (or a bare "statement")
//	<dataset>/<problem>/constraints.{json,txt}      (or a bare "constraints")
//	<dataset>/<problem>/solutions/<id>.{cpp,cc,cxx}
//	<dataset>/problems/<problem>/...                (official staging layout)
//	<dataset>/solutions/<problem>/<id>.{cpp,cc,cxx} (official staging layout)
//	<dataset>/labs/<lab>/...                        (lab-style container)
//
// Statement and constraint names may carry a suffix (for example
// "statement-v2.md") as long as the extension is one of the documented ones.
// Multiple candidates resolve deterministically: an exact base name first, then
// .md, then .txt, then lexicographic order.
//
// Constraints are either JSON (an array of {id,text,...} or an object with a
// "constraints" array) or one constraint per line. A line may start with an
// explicit identifier ("C1: ...", "C1\t...") or an identifier containing a
// digit followed by whitespace; otherwise the line is the text and a stable
// "C<n>" identifier is assigned by line order.
//
// # Determinism and memory
//
// Problems, solutions and constraints are sorted by identifier. The output is
// written through a buffered temp file and renamed into place. One problem's
// constraints and one solution source (capped at 65,536 bytes, the documented
// PA#3 source limit) are held at a time; the whole dataset is never loaded.
package checker
