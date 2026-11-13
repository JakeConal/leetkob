# LeeTKOB judge and checker messages — contract, major version 1 (draft for review)

CSC13114 · OpenSpec `complete-leetkob-product-workflow` task 1.3 · drafted 13 Nov 2026

This directory specifies the messages exchanged between the trusted backend and the two processing boundaries: the isolated execution service that compiles and runs submissions, and the checker core shared by the worker and the evaluation command. [`v1/messages.schema.json`](v1/messages.schema.json) defines every shape in JSON Schema 2020-12. [`v1/fixtures/messages.json`](v1/fixtures/messages.json) holds messages that must be accepted and messages that must be rejected. If code and this contract disagree, the code is wrong.

Status: **draft, not reviewed, not implemented.** It is reviewed together with the HTTP contract in [`../v2/README.md`](../v2/README.md) and shares its conventions for identifiers, time, units, and money.

## 1. Messages

| `type`                  | From → to                          | Purpose                                                                 |
| ----------------------- | ---------------------------------- | ----------------------------------------------------------------------- |
| `leetkob.judge.request` | Worker → execution service         | Compile one source and run it against the staged tests under one policy |
| `leetkob.judge.result`  | Execution service → worker         | Aggregate and per-test verdicts, or an infrastructure failure           |
| `leetkob.check.request` | Worker or evaluation CLI → checker | Check one Accepted source against the active constraints                |
| `leetkob.check.result`  | Checker → worker or evaluation CLI | One finding per constraint with evidence, or an infrastructure failure  |

The messages are transport-neutral JSON. Jobs are durable database rows; a message is the body of one job attempt and of its result. Nothing here requires a broker.

## 2. Envelope

Every message has the same top-level members.

| Member          | Rule                                                                                                                                           |
| --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `type`          | One of the four names above.                                                                                                                   |
| `version`       | `MAJOR.MINOR`. This contract is major version 1.                                                                                               |
| `messageId`     | UUID of this message. A consumer discards a `messageId` it has already committed.                                                              |
| `correlationId` | Opaque UUID that links acceptance, judging, checking, and review in telemetry. It is not a student, submission, or class identifier.           |
| `createdAt`     | UTC instant with milliseconds and `Z`.                                                                                                         |
| `job`           | `id`, `attempt`, `leaseExpiresAt`. `attempt` is the fencing token of the job claim.                                                            |
| `subject`       | `labId`, `submissionId`, `gradingGeneration`, `testRevision`, `sourceSha256`: the exact inputs the work is computed from. A result repeats it. |
| `payload`       | Defined per type in section 4.                                                                                                                 |

## 3. Versioning and compatibility

- A producer emits only documented members. The schema is closed so that a typo or an undocumented member fails validation at the producer.
- A consumer ignores members it does not know **within** major version 1 and rejects every other major version with failure code `unsupported-version`. It never guesses.
- A minor version adds optional members or enumeration values that existing consumers may ignore. The schema file is updated in place and the change is listed in section 8.
- Removing or renaming a member, changing a meaning or a unit, or adding a value an existing consumer must understand is a new major version in a new directory (`v2/`). Both majors are accepted while jobs of the old one can still exist.
- These messages are versioned independently of the HTTP contract.

## 4. Payloads

### 4.1 `leetkob.judge.request`

`executionPolicy` is identical to the HTTP `ExecutionPolicy`: base units, a fixed compile profile `gxx-std-cpp17-o2`, and a wall limit of three times the CPU limit. `source` is the original bytes in base64 with their decoded length. `tests` lists each test in stable order by digest; test bytes are staged by digest and never embedded.

The request is everything the execution boundary receives. It has no credential, environment, account, class, or student member, and the schema rejects one.

### 4.2 `leetkob.judge.result`

`payload.outcome` is `completed` or `infrastructure-failure`.

A completed result has `verdict`, `compile`, `tests`, and `executor` (the isolation backend and image digest).

| Observation                                           | Result                                                        |
| ----------------------------------------------------- | ------------------------------------------------------------- |
| Compilation fails                                     | `compile.status` `error`, verdict `CE`, no tests              |
| Compilation exceeds 10,000 ms                         | `compile.status` `timeout`, verdict `CE`, no tests            |
| Tokens equal the expected tokens, ignoring whitespace | Test `AC`                                                     |
| Tokens differ                                         | Test `WA`                                                     |
| CPU or wall limit exceeded                            | Test `TLE`                                                    |
| Memory limit exceeded                                 | Test `MLE`, or `RE` on a backend documented as unable to tell |
| More than 1,048,576 output bytes, or abnormal exit    | Test `RE`                                                     |
| Every test `AC`                                       | Verdict `AC`                                                  |
| Otherwise                                             | Verdict of the first non-`AC` test in stable order            |

A denied process creation is not a verdict of its own; the program's resulting behavior decides.

An infrastructure failure has `failure.code` and `failure.retryable` and no verdict. It is never shown to a student as a verdict and never becomes a test failure. Retries are bounded; an exhausted job stays visible as failed and blocks publication readiness.

| `failure.code`          | Meaning                                                         |
| ----------------------- | --------------------------------------------------------------- |
| `unsupported-version`   | The consumer does not implement the message's major version     |
| `invalid-message`       | The message violates the schema                                 |
| `input-digest-mismatch` | Staged source or test bytes do not match the digests in the job |
| `staging-failed`        | Task files could not be prepared                                |
| `sandbox-unavailable`   | The isolation backend could not start or enforce a limit        |
| `lease-expired`         | The attempt outlived its lease                                  |
| `internal`              | Any other failure                                               |

### 4.3 `leetkob.check.request`

The payload has `statementMarkdown`, the active `constraints` (`id`, `key`, `revision`, `text`), the original `source`, and `redactionTerms`.

- There is no deduction amount, test content, account, or credential, and the schema rejects one.
- `redactionTerms` are the known enrollment names and student codes that sanitization must remove from every outbound field. They are never logged and never sent to a provider.
- Only the sanitized form of the source may leave the checker. The original stays inside it to restore evidence lines.

### 4.4 `leetkob.check.result`

A completed result has `checker` (`checkerVersion`, `promptVersion`, `modelId`), `findings`, and `usage`.

- There is exactly one finding per requested constraint, with `source` `deterministic` or `model` and `verdict` `pass`, `violation`, or `unsure`.
- A violation has at least one evidence line `{ "line", "text" }`, 1-based, whose text is the original source line. A pass has none.
- An unsure has a specific `unsureCause`. An unsure never creates a deduction.
- A finding has no grade, score, or deduction member, and the schema rejects one. Deductions come only from the constraint revision and a TA decision.
- `usage.modelCalls` is at most 2: one call and one schema retry. `settledMicroUsd` is `null` while provider usage is unknown; the reservation then stays charged against the lab budget.

| `unsureCause`            | Raised when                                                                 |
| ------------------------ | --------------------------------------------------------------------------- |
| `preparation-failed`     | Sanitization could not preserve analysis and line mapping; nothing was sent |
| `unsupported-resolution` | A deterministic rule met an alias, macro, or wrapper it cannot resolve      |
| `schema-invalid`         | The response failed the schema twice                                        |
| `evidence-mismatch`      | Cited evidence does not match the original line                             |
| `provider-timeout`       | The provider did not answer in time                                         |
| `provider-failure`       | The provider returned an error                                              |
| `budget-exhausted`       | The lab budget cannot cover the reservation                                 |
| `pricing-unknown`        | Price or token bound is unknown, so dispatch was refused                    |

Provider, schema, evidence, and budget problems are findings, not failures. `infrastructure-failure` is reserved for the checker process itself failing; the job is then retried without consuming another schema retry or resetting the budget.

## 5. Accepting a result

The worker commits a result and the next stage's intent in one transaction, and only when all of these hold:

1. The message validates and its major version is supported.
2. `job.attempt` is the highest attempt issued for `job.id`. A lower attempt is discarded.
3. No result for this job has been committed. A duplicate delivery changes nothing.
4. Every member of `subject` equals the lab's current state: the submission is the counted one where that matters, `gradingGeneration` and `testRevision` are active, and `sourceSha256` matches. For a check result every `constraintRevision` is active.

A result that fails rule 4 is stored as history. It never updates a current view, satisfies readiness, or restores a superseded submission. Only a counted submission whose current verdict is `AC` produces a check request.

## 6. Privacy and trust

- Neither request type carries a credential. The execution service and the model provider never receive a database, storage, or model credential through a message.
- A judge request and result carry no student or class identity.
- Message bodies are not written to operational logs. Telemetry uses `correlationId`, `job.id`, and counts.
- Source in a message is untrusted data. Nothing in a message is executed or interpreted as an instruction by the worker.

## 7. Evaluation prediction file

The checker command writes one JSON line per (solution, constraint) pair in the format owned by the [official evaluation contract](https://github.com/fithcmus/awad-leetkob-eval/blob/552c811db2d35a23d641bc4fdffc1ac120be1a28/README.md): `solution`, `constraint`, `verdict`, and `evidence` of `{ "line", "text" }`. The schema's `Prediction` definition reproduces that format so the CLI adapter can be validated. It is not an envelope and has no version member; if the official format changes, the official contract wins.

The CLI adapter maps a `check.result` finding to a prediction by copying `verdict` and `evidence` and using the dataset's solution and constraint identifiers. It never reads labels.

## 8. Changes

| Version | Date        | Change                    |
| ------- | ----------- | ------------------------- |
| 1.0     | 13 Nov 2026 | First draft, under review |
