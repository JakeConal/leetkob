// Package checker is the shared, dataset-independent constraint checker used
// by the offline marking command (cmd/checker) and by the judging worker.
//
// # Prediction contract
//
// The package discovers problems, statements, constraints and solution
// sources from the documented dataset layout and writes one deterministic JSONL
// prediction per solution/constraint pair, matching
// docs/contract/schemas/checker-prediction.v1.json. Discovery is generic: no
// problem-ID table, per-problem switch, prompt, label or scorer read exists.
// Dataset-level metadata such as labels.jsonl, eval.mjs, reports and test data
// is never opened, even when it sits next to the inputs.
//
// # Verdict selection
//
// Ban-list constraints ("Do not use ..." / "Do not call ..." with backticked
// names) are decided only by the analyzer boundary, which resolves C++
// declarations rather than text. Comments, string literals, harmless includes
// and student-defined names therefore cannot become a wrongful deduction. Any
// tooling, protocol or evidence failure becomes unsure.
//
// Other constraints are semantic. They are sent to the guarded model port only
// when MODEL_OFF is not set, sanitization succeeded, a semantic port exists and
// a conservative priced reservation succeeds. Sanitization removes comments
// and known identities while preserving line numbers; unsafe sanitization
// produces unsure without transmission. Ban-list constraints are never sent to
// the model.
//
// # Offline operation
//
// MODEL_OFF=1 is selected before any provider client, credential read or
// budget reservation. Every semantic pair is then unsure, ban-list pairs are
// still decided by the bundled analyzer, and the summary reports zero model
// calls. The checker image embeds the analyzer executable and runs with
// --network none, memory/PID limits, no keys and no Docker socket.
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
