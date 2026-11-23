# Unfamiliar checker fixture

Deliberately invented, non-upstream problem IDs in the documented prediction
input layout. This fixture stands in for PA3-EVAL-01: the checker must produce
predictions without any code, table or prompt change for these IDs.

The dataset-level `labels.json`, `eval.mjs` and `report.json` files are invalid
on purpose. Any code that opens or parses them fails the test; the checker must
ignore them completely.

Problem containers are exercised in three forms:

- `problems/<id>/...` with in-problem `solutions/` (official staging layout)
- a direct `<dataset>/<id>/...` problem directory
- `labs/<id>/...` with `statement-final.md` and a tab-separated constraints file
- top-level `solutions/<id>/...` sources alongside in-problem solutions

Expected pairs are derived in `internal/checker/predict_test.go` from discovery,
not from this file.
