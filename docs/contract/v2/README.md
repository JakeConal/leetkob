# LeeTKOB production API — contract, version 2.0 (draft for review)

CSC13114 · OpenSpec `complete-leetkob-product-workflow` task 1.3 · drafted 13 Nov 2026

This directory is the specification of the production API. [`openapi.yaml`](openapi.yaml) defines every operation and shape. This file defines the rules a schema cannot express: evaluation order, authorization, idempotency, revisions, and time and decimal handling. [`fixtures/`](fixtures) holds language-neutral examples that every implementation must reproduce. If code and this contract disagree, the code is wrong.

The judge and checker message envelopes are specified separately in [`../messages/README.md`](../messages/README.md). The unversioned PA#2 contract in [`../publish-lab.md`](../publish-lab.md) is unchanged and stays in force for its own process (section 2).

## 1. Status

| Item                                 | State                                                                                             |
| ------------------------------------ | ------------------------------------------------------------------------------------------------- |
| Contract version                     | `2.0.0-draft.1`                                                                                   |
| Contract review                      | **Pending.** No reviewer has approved this draft. Record the reviewers and date here on approval. |
| Product-owner answer on units (D01)  | **Pending.** Section 7 states the proposed rule and what changes if the answer differs.           |
| Implementation                       | None. No `/v2` route exists in any process.                                                       |
| Fixtures and executable checks       | Present. `npm test` runs `apps/api/test/contract/production-contract.test.ts`.                    |
| Operations of enhancement phase 9    | Not contracted. Section 10 lists them. Each needs a contract revision before implementation.      |
| Frontend type generation, drift gate | Specified in section 8. Installed by tasks 1.2, 1.7, and 2.1, not by this task.                   |

No endpoint in this document may be implemented until the review row says approved. Section 7 lists every decision a reviewer is asked to confirm.

## 2. Scope and separation from v1.1

Version 2 covers the ten production resource families of the design, all under the `/v2` prefix: `/v2/auth/*`, `/v2/classes/*`, `/v2/labs/*`, `/v2/labs/{labId}/submissions`, `/v2/submissions/{submissionId}`, `/v2/labs/{labId}/reviews`, `/v2/labs/{labId}/publish`, `/v2/labs/{labId}/grades`, `/v2/deductions/{deductionId}/appeals`, and `/v2/events`. `GET /health` keeps its existing unversioned behavior.

The unversioned `POST /labs/{labId}/publish` operation keeps its v1.1 contract and its test suite. The two versions never share a process or data.

| Aspect                    | v1.1 (`../publish-lab.md`)                                | v2 (this contract)                                                                 |
| ------------------------- | --------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Route                     | `POST /labs/{labId}/publish`                              | `POST /v2/labs/{labId}/publish`                                                    |
| Process                   | Compatibility and demonstration process, seeded in memory | Production process. The unversioned route is not registered: `404 route-not-found` |
| Identity                  | Bearer claim with embedded role and class IDs             | Session cookie. Role and membership are read from current server state per request |
| Replay and authorization  | Replay is evaluated before role and ownership             | Role and ownership are evaluated before replay                                     |
| Stored idempotent results | `200`, `403`, `404`, `409`                                | `200` and `409` only                                                               |
| Readiness                 | Pending-review count                                      | Finalized admissions, terminal judging and checking, sampling, required decisions  |
| State change              | Publication metadata only                                 | Publication metadata, grade snapshot, audit event, and notification intents        |
| `publishedBy`             | Opaque subject string                                     | User UUID                                                                          |

Nine problem types are shared by both versions and keep the same type URI, status, and title: `invalid-request`, `authentication-required`, `ta-role-required`, `lab-not-found`, `route-not-found`, `lab-still-open`, `reviews-pending`, `lab-already-published`, and `internal-error`. Changing one of them is a breaking change to both contracts.

## 3. Conventions

### 3.1 Transport

- Every path starts with `/v2/`, has no trailing slash, and is served from the same origin as the web application.
- JSON bodies are UTF-8. Success responses use `application/json; charset=utf-8`; problems use `application/problem+json; charset=utf-8`.
- Every response carries `Cache-Control: no-store`.
- A request body must use the documented media type; a `charset=utf-8` parameter is allowed. Any other type returns `415 unsupported-media-type`.
- Request objects are closed. An unknown or duplicate member, an unknown query parameter, or a duplicated header returns `400 invalid-request`. There are no undocumented optional members.
- Clients ignore unknown response members.
- Body limits are enforced while reading. A longer body returns `413 payload-too-large` with `maximumBytes`.

| Body                                 | Limit in bytes |
| ------------------------------------ | -------------: |
| JSON, default                        |         16,384 |
| JSON for `createLab` and `updateLab` |        524,288 |
| Submission source                    |         65,536 |
| Roster CSV                           |         65,536 |
| Test bundle ZIP                      |     10,485,760 |

### 3.2 Identifiers

- Resource identifiers are UUIDs. Responses use canonical lowercase; requests are accepted case-insensitively.
- A constraint also has a `key` such as `C1`, unique within its lab, which people and evaluation datasets use.
- Identifiers are opaque. A caller never learns from an identifier or an error whether a resource outside their access exists (section 4.3).

### 3.3 Time

- Every instant is an RFC 3339 string in UTC with exactly three fractional digits and the `Z` suffix: `2026-11-25T15:00:00.000Z`. No other offset or precision is accepted in a request.
- The server clock is the only authority. Client timestamps are never read.
- The server compares instants at its internal precision. A value written to a response is truncated to milliseconds, never rounded, so an accepted `receivedAt` never displays as the deadline.
- Durations are integers whose member name ends in the unit: `cpuTimeLimitMs`, `retryAfterSeconds`.
- The interface renders local time with an explicit time zone. The API never sends local time.

### 3.4 Decimals and money

- Points (grades, deductions) are **strings** with exactly two fractional digits, from `"0.00"` through `"10.00"`: `"3.00"`, `"0.25"`. A signed adjustment ranges from `"-10.00"` through `"10.00"`; zero is `"0.00"`, with no `+` sign and no negative zero.
- Requests must use the same canonical form. `3`, `3.0`, and `3.000` return `400 invalid-request`.
- Implementations compute in integer hundredths. Points never pass through binary floating point.
- Money is an integer count of micro-USD in members ending in `MicroUsd`. USD 0.25 is `250000`.

### 3.5 Sizes and units

Every size is an integer number of bytes and every limit uses binary units (D01, section 7).

| Limit                       | Value                                                                    |
| --------------------------- | ------------------------------------------------------------------------ |
| Source                      | 65,536 bytes (64 KiB) of valid UTF-8, at least 1 byte                    |
| CPU time per test           | 1,000 ms by default; 1,000 to 5,000 ms in whole seconds                  |
| Wall time per test          | Always three times the CPU limit                                         |
| Memory                      | 268,435,456 bytes (256 MiB) by default; 64 to 512 MiB in whole mebibytes |
| Compilation time            | 10,000 ms, fixed                                                         |
| Processes including threads | 16, fixed                                                                |
| Standard output per test    | 1,048,576 bytes (1 MiB), fixed                                           |
| Compiler                    | `g++ -std=c++17 -O2`, fixed, named `gxx-std-cpp17-o2`                    |

### 3.6 Pagination

List operations that can grow take `limit` (1 to 100, default 50) and `cursor`, and return `{ "items": [...], "nextCursor": "<opaque>" | null }`. The order is stable and documented per operation. A cursor is valid only for the same operation and filters; an invalid cursor returns `400 invalid-request`.

### 3.7 Revision vocabulary

| Member                 | Meaning                                                                                                                            |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `revision`             | Positive integer of one mutable resource (user, lab, constraint, finding, appeal). It increases by one on every committed change.  |
| `expectedRevision`     | The `revision` the caller last read, sent with every mutation of an existing resource.                                             |
| `constraintRevision`   | The `revision` of a constraint at the time a finding or deduction was produced. Constraint content is immutable per revision.      |
| `testRevision`         | Sequence number of an immutable test bundle within its lab.                                                                        |
| `gradingGeneration`    | Lab-level counter. It increases whenever a counted submission, the active test revision, or an active constraint revision changes. |
| `gradeVersion`         | Append-only version of one student's lab grade. Version 1 is the publication snapshot.                                             |
| `expectedGradeVersion` | The `gradeVersion` the TA last read, sent with an adjustment.                                                                      |
| `acceptanceSequence`   | Durable server-assigned order of accepted submissions within a lab.                                                                |

Section 5.4 defines how these are checked.

## 4. Authentication and authorization

### 4.1 Sessions

| Cookie                | Content                               | Lifetime   | Attributes                                         |
| --------------------- | ------------------------------------- | ---------- | -------------------------------------------------- |
| `__Host-lk_access`    | Signed access token: subject, session | 15 minutes | `Secure; HttpOnly; SameSite=Lax; Path=/`           |
| `__Secure-lk_refresh` | Opaque one-time refresh token         | 7 days     | `Secure; HttpOnly; SameSite=Strict; Path=/v2/auth` |

- The session cookie is the only way to authenticate. An `Authorization` header and any role or class header are ignored.
- Tokens never appear in a response body, a URL, a log, or client-readable storage. The server stores a refresh token only as a hash.
- `refreshSession` consumes the presented refresh token and sets a new pair. Presenting a consumed token revokes the whole session family. A session family ends seven days after login; rotation does not extend it.
- `logOut` revokes the family and clears both cookies.
- Missing, malformed, tampered, expired, and revoked sessions all return the same `401 authentication-required`.
- The access token names a subject and a session. It never grants a role or a class.

### 4.2 Cross-site request protection

Every request other than `GET` must carry an `Origin` header equal to the configured web origin. Every such request made with a session must also carry `X-CSRF-Token` equal to the `csrfToken` of that session, which `registerAccount`, `logIn`, `refreshSession`, and `getSession` return. A failure returns `403 csrf-rejected` and changes nothing. `registerAccount`, `logIn`, and `refreshSession` rely on the `Origin` check alone because no token exists yet.

### 4.3 Authorization

- The role (`admin`, `ta`, `student`) and class memberships are read from current server state on every request and before every event delivery. Removing a TA assignment or an enrollment takes effect on the next request even while the access token is valid.
- Registration always creates a student. Only an administrator changes a role (`setUserRole`), and the change is audited. The first administrator is created by a documented operator action, not through the API.
- An administrator manages classes and roles. Being an administrator grants no access to labs, submissions, reviews, or grades.
- A TA reads and changes only resources of classes they are currently assigned to. A student reads only opened labs of classes they are enrolled in, and only their own submissions, grades, and appeals.
- **Concealment.** A resource that does not exist and a resource outside the caller's access return the same `404` problem of that resource type, with the five base members only. This applies to direct identifiers, lists, filters, downloads, and events.
- A role failure (`403`) is reported before any resource is looked up, so it reveals nothing about the resource.

Each operation in `openapi.yaml` states its `actors` and `scope` under `x-leetkob`.

## 5. Cross-cutting semantics

### 5.1 Problems

Every error is an RFC 9457 object: `type`, `title`, `status`, `detail`, `instance`, plus the extension members listed below. `type` is `https://leetkob.example/problems/<suffix>`; clients branch on it. `title` is stable. `status` equals the HTTP status. `instance` is the request path without its query string. `detail` is a safe sentence for people and is not contractual; it never contains source code, test data, student identity, credentials, stack traces, storage details, or another class's identifiers.

| Type suffix                     | Status | Title                         | Extension members                                                                                                       | When                                                                                                    |
| ------------------------------- | -----: | ----------------------------- | ----------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `invalid-request`               |    400 | Invalid request               | optional `errors[]` of `in`, `name`, `code`                                                                             | Malformed identifier, header, query, or body; unknown member; value out of range; inconsistent request. |
| `evidence-invalid`              |    400 | Evidence invalid              | `invalidLines`                                                                                                          | A cited evidence line does not match the original source.                                               |
| `test-bundle-invalid`           |    400 | Test bundle invalid           | `violations`                                                                                                            | Unsafe path, link, unmatched pair, or exceeded expansion or entry limit. Nothing is activated.          |
| `authentication-required`       |    401 | Authentication required       | —                                                                                                                       | No valid session.                                                                                       |
| `invalid-credentials`           |    401 | Invalid credentials           | —                                                                                                                       | Login failed. Unknown email and wrong password are indistinguishable.                                   |
| `csrf-rejected`                 |    403 | Cross-site request rejected   | —                                                                                                                       | Missing or wrong `Origin` or `X-CSRF-Token` on a mutation.                                              |
| `admin-role-required`           |    403 | Administrator role required   | —                                                                                                                       | The caller is not an administrator.                                                                     |
| `ta-role-required`              |    403 | TA role required              | —                                                                                                                       | The caller is not a TA.                                                                                 |
| `student-role-required`         |    403 | Student role required         | —                                                                                                                       | The caller is not a student.                                                                            |
| `route-not-found`               |    404 | Route not found               | —                                                                                                                       | No operation for this method and path, including every unversioned path. Universal.                     |
| `user-not-found`                |    404 | User not found                | —                                                                                                                       | No such account, for administrator and roster operations.                                               |
| `class-not-found`               |    404 | Class not found               | —                                                                                                                       | Absent class or one outside the caller's access.                                                        |
| `invitation-not-found`          |    404 | Invitation not found          | —                                                                                                                       | Unknown, expired, or revoked invitation.                                                                |
| `lab-not-found`                 |    404 | Lab not found                 | —                                                                                                                       | Absent lab, lab of another class, or a lab that has not opened for a student.                           |
| `constraint-not-found`          |    404 | Constraint not found          | —                                                                                                                       | Absent or deleted constraint of an accessible lab.                                                      |
| `submission-not-found`          |    404 | Submission not found          | —                                                                                                                       | Absent submission or one the caller may not read.                                                       |
| `finding-not-found`             |    404 | Finding not found             | —                                                                                                                       | Absent finding of an accessible lab.                                                                    |
| `grade-not-found`               |    404 | Grade not found               | —                                                                                                                       | No grade visible to the caller: unpublished for a student, or a student outside the frozen roster.      |
| `deduction-not-found`           |    404 | Deduction not found           | —                                                                                                                       | Absent deduction, another student's deduction, or an unpublished one.                                   |
| `appeal-not-found`              |    404 | Appeal not found              | —                                                                                                                       | Absent appeal of an accessible deduction.                                                               |
| `registration-rejected`         |    409 | Registration rejected         | —                                                                                                                       | Registration cannot be completed with these details.                                                    |
| `role-change-not-allowed`       |    409 | Role change not allowed       | —                                                                                                                       | The account's memberships conflict with the requested role or membership.                               |
| `class-capacity-reached`        |    409 | Class capacity reached        | `capacity`                                                                                                              | The class already has 100 students.                                                                     |
| `revision-conflict`             |    409 | Revision conflict             | `conflicts[]` of `resource`, `id`, `currentRevision`                                                                    | An `expectedRevision` is stale or names a superseded resource.                                          |
| `idempotency-key-reused`        |    409 | Idempotency key reused        | —                                                                                                                       | The key was completed with a different payload.                                                         |
| `lab-configuration-locked`      |    409 | Lab configuration locked      | `lockedFields`, `opensAt`                                                                                               | Schedule, execution policy, or test change after the lab opened.                                        |
| `constraint-change-not-allowed` |    409 | Constraint change not allowed | `rule`                                                                                                                  | Addition, deduction increase, or unconfirmed text change after opening.                                 |
| `submission-deadline-passed`    |    409 | Submission deadline passed    | `submissionDeadline`                                                                                                    | The complete request arrived at or after the deadline.                                                  |
| `group-decision-not-allowed`    |    409 | Group decision not allowed    | `findingIds`                                                                                                            | A group contains a finding that is not a current deterministic violation of the constraint.             |
| `lab-still-open`                |    409 | Submissions still open        | `submissionDeadline`                                                                                                    | Publication before the deadline.                                                                        |
| `lab-processing-incomplete`     |    409 | Lab processing incomplete     | `admissionsInFlight`, `judgingIncomplete`, `judgingFailed`, `checkingIncomplete`, `checkingFailed`, `samplingFinalized` | Publication while admissions, judging, checking, or sampling are not final.                             |
| `reviews-pending`               |    409 | Reviews pending               | `pendingReviews`                                                                                                        | Publication while required decisions are missing.                                                       |
| `lab-already-published`         |    409 | Lab already published         | `labId`, `publishedAt`, `publishedBy`                                                                                   | Publication with a new key, or any authoring, review, or constraint change after publication.           |
| `grades-not-published`          |    409 | Grades not published          | —                                                                                                                       | A TA adjustment before publication.                                                                     |
| `appeal-already-exists`         |    409 | Appeal already exists         | `appealId`                                                                                                              | The deduction already has an appeal created with another key.                                           |
| `appeal-window-closed`          |    409 | Appeal window closed          | `appealDeadline`                                                                                                        | Appeal at or after `publishedAt` plus seven days.                                                       |
| `appeal-already-resolved`       |    409 | Appeal already resolved       | —                                                                                                                       | Resolution of an appeal that is not pending.                                                            |
| `payload-too-large`             |    413 | Payload too large             | `maximumBytes`                                                                                                          | The body exceeds the operation's limit.                                                                 |
| `unsupported-media-type`        |    415 | Unsupported media type        | —                                                                                                                       | The body's media type is not the documented one.                                                        |
| `rate-limited`                  |    429 | Too many requests             | `retryAfterSeconds`, header `Retry-After`                                                                               | A documented attempt limit was exceeded. The response does not depend on whether an account exists.     |
| `internal-error`                |    500 | Internal server error         | —                                                                                                                       | Unexpected failure. `detail` is one fixed generic sentence. Universal.                                  |
| `service-unavailable`           |    503 | Service unavailable           | —                                                                                                                       | A required dependency is unavailable. Nothing was committed. Universal.                                 |

Every failure leaves state unchanged, except that a failed login counts toward its rate limit and a reused refresh token revokes its session family.

### 5.2 Evaluation order

The server evaluates a request in this order and stops at the first result.

1. Method and path, otherwise `route-not-found`.
2. Attempt limit where the operation documents `rate-limited`.
3. Session, otherwise `authentication-required`.
4. `Origin` and CSRF token for a mutation, otherwise `csrf-rejected`.
5. Input that needs no stored state: identifiers, query, headers including `Idempotency-Key`, media type (`415`), size (`413`), and body shape (`400`).
6. Role, otherwise the `*-role-required` problem.
7. Resource existence and current access, otherwise the concealed `*-not-found` problem.
8. For a keyed operation, the stored result for the key (section 5.3).
9. Publication state, otherwise `lab-already-published` for an operation that a published lab no longer allows.
10. `expectedRevision`, otherwise `revision-conflict`.
11. The remaining rules of the operation in its documented order, including `evidence-invalid` and `test-bundle-invalid`, which need stored state.
12. One atomic commit, then the response.

Submission acceptance refines steps 5 to 8 because its body is read after access is established (section 6.4).

### 5.3 Idempotency

Each operation declares one mode under `x-leetkob.idempotency`.

| Mode       | Meaning                                                                                                                                           |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `required` | The request carries `Idempotency-Key`. Used by `createSubmission`, `publishLab`, and `createAppeal`.                                              |
| `revision` | The request carries `expectedRevision`. A retry after a lost response returns `revision-conflict`; the client refetches and sees its own change.  |
| `natural`  | Repeating the request has no further effect because the target is identified by its content (a membership, an assignment, a revocation).          |
| `none`     | Reads, session operations, and creations for which a duplicate is harmless and visible (a class, a lab draft, a constraint draft, an invitation). |

Rules for `required`:

- `Idempotency-Key` is exactly one header value of 8 to 128 visible ASCII characters (`!` through `~`). Anything else returns `400 invalid-request`.
- The scope is `(authenticated user, operation, target resource, key)`. The target is the lab for a submission and a publication and the deduction for an appeal.
- The server stores the first **completed result reached after authorization**: the success response and every `409` of the operation, except `idempotency-key-reused`. It never stores `400`, `401`, `403`, `404`, `413`, `415`, `429`, or any `5xx`.
- A later request in the same scope returns the stored status, media type, and body exactly. Mutable state is not evaluated again. A stored rejection stays a rejection after its cause is resolved; a deliberate new attempt needs a new key.
- Current authorization (steps 3 to 7) is evaluated before the stored result is read. A caller who lost access receives the concealed `404`, not the stored result.
- A submission's fingerprint is the SHA-256 of its exact body bytes; an appeal's is its exact `explanation`. The same key with a different fingerprint returns `409 idempotency-key-reused`. A publication has no payload.
- Two requests with the same key that overlap are serialized; the second returns the first one's result. Requests with different keys for the same target are serialized by the target's lock.
- A completed key is retained for at least 24 hours. The key of an accepted submission or a created appeal is retained as long as that resource.
- A client reuses the key when it retries after a timeout or a dropped connection, and creates a new key for every new deliberate action.

### 5.4 Revisions

- Every mutation of an existing resource states the revision it was based on. A missing `expectedRevision` returns `400 invalid-request`.
- If the stated revision is not current, nothing is written and the response is `409 revision-conflict`. `conflicts` lists every stale resource the request named with its `currentRevision`, or `null` when that resource was superseded and is no longer current. The server never merges.
- A group request is all-or-nothing: one stale item rejects every item.
- A committed constraint revision, deletion, or test activation, and any change of a counted submission, increases the lab's `gradingGeneration`. Findings and decisions of an affected constraint or test result are superseded, including after a deduction-only edit. They stay readable as history and never satisfy readiness.
- Judge and checker results name the generation, test revision, and source digest they were computed from. A result whose inputs are no longer current is stored as history and changes nothing current (see the messages contract).
- Publication, constraint revision, review decisions, grade adjustment, and appeal resolution take the same lab lock, so one commits first and the other observes the result.

## 6. Resource families

`openapi.yaml` is authoritative for paths, shapes, and the problems of each operation. This section adds the rules between operations.

### 6.1 `/v2/auth`

- `registerAccount` creates a student account and a session. The request has no role member.
- `logIn` compares the email case-insensitively. Failure is always `invalid-credentials` after comparable work, whether or not the account exists.
- Proposed attempt limits: 10 failed logins per 15 minutes per client address and email, 100 per 15 minutes per client address, 5 registrations per hour per client address, 10 invitation redemptions per 15 minutes per user. Counters are durable.
- `findUserByEmail` and `setUserRole` are administrator operations. `setUserRole` rejects a change that would leave a TA assignment or a student enrollment inconsistent with the new role, and an administrator cannot change their own role.

### 6.2 `/v2/classes`

- An administrator creates classes and assigns TAs. A TA or student may belong to several classes.
- A student joins by redeeming an invitation code or by a roster entry made by an assigned TA. Both are repeat-safe: a second enrollment returns the existing membership.
- A roster entry needs an existing student account. `importRoster` takes `email,studentCode` rows, applies each row independently in file order, and reports every row with `enrolled`, `unchanged`, or `rejected` and a code.
- A class never has more than 100 students, also under concurrent enrollment. The request that would exceed it returns `class-capacity-reached`; a roster row reports `class-capacity-reached`.
- Removing a member never changes a lab roster that was frozen at an earlier submission deadline.

### 6.3 `/v2/labs`

- A lab's `status` is derived: `scheduled` before `opensAt`, `open` while `opensAt <= now < submissionDeadline`, `closed` from the deadline, `published` after publication. `opensAt` must be earlier than `submissionDeadline`.
- A student reads a lab, its constraints, deductions, sample tests, and limits from `opensAt`. Before that the lab is concealed. The student projection `StudentLab` has no hidden-test, budget, or review data.
- From `opensAt`, the schedule, the execution policy, and the test bundle are locked (`lab-configuration-locked`). Title and statement stay editable until publication.
- A test bundle is a ZIP of `sample/<name>.in` with `sample/<name>.out` and `hidden/<name>.in` with `hidden/<name>.out`. It is rejected as a whole for an absolute path, parent traversal, a link, an entry outside those forms, an unmatched pair, more than 1,000 entries, or more than 104,857,600 expanded bytes. Validation happens in isolated staging and stops at the first exceeded limit.
- Constraints follow the decision table `constraintEdits` in `fixtures/vectors.json`: any change before opening; from opening until publication no addition and no deduction increase, a text change only with `preservesMeaning: true`, and a `reason` for every change; nothing after publication.
- `getLabReadiness` reports counts and `blockers` only. `publishable` is true exactly when `blockers` is empty.

### 6.4 `/v2/labs/{labId}/submissions` and `/v2/submissions/{submissionId}`

`createSubmission` accepts the raw source as `text/plain` and evaluates in this order:

1. Route, attempt limit, session, CSRF.
2. Lab identifier, `Idempotency-Key`, media type, and a declared length above the limit.
3. Student role, then enrollment and `opensAt <= now`, otherwise `lab-not-found`.
4. An admission lease is registered. Deadline finalization and publication wait for every lease registered before the deadline to commit or abort. A lease has a short timeout and is fenced: an expired lease can never commit.
5. The body is read up to 65,536 bytes. A longer body returns `payload-too-large`. `receivedAt` is stamped when the last byte has arrived.
6. An empty body or invalid UTF-8 returns `invalid-request`.
7. A stored result for the key: the same bytes replay it, different bytes return `idempotency-key-reused`.
8. `receivedAt >= submissionDeadline` returns `submission-deadline-passed`.
9. One transaction stores the source, the submission, its `acceptanceSequence`, the counted selection, and the judge job, then releases the lease. Only then `201` is sent.

Because step 7 precedes step 8, a retry after the deadline of a submission accepted before it returns the original `201`. If storage fails, nothing is acknowledged, counted, or stored for the key.

- The counted submission is the accepted one with the greatest `(receivedAt, acceptanceSequence)`, whatever its verdict. It is provisional before the deadline and final afterwards. Worker completion order never changes it.
- `judge.status` moves from `queued` to `running` to `completed`, or to `infrastructure-failed` when retries are exhausted. The latter is an operational failure that blocks readiness and is never shown as a verdict.
- The aggregate verdict is `CE` when compilation fails or times out, otherwise the first non-`AC` test in stable order, otherwise `AC`.
- A submission and its source are visible to its author and to TAs assigned to its class. No projection contains test input, expected output, actual output, or findings.

### 6.5 `/v2/labs/{labId}/reviews`

- For the current grading generation the required queue is every violation, every unsure, and `ceil(P / 10)` pass findings drawn at random without replacement, where `P` counts pass findings and a finding is one counted Accepted submission paired with one active constraint. The sample is stored; reading the queue never redraws it.
- A decision is `pass` or `violation`, never a default. `violation` needs at least one evidence line whose `text` equals the original line `line` after trimming leading and trailing whitespace from both and nothing else (D09). The stored evidence is the original line restored from the source.
- `reason` is required when the decision differs from the checker verdict or the checker verdict is `unsure`.
- A TA may decide any current finding, including an unsampled pass (`scope=all`).
- `decideFindingGroup` is the only group action. It confirms current deterministic violations of one constraint, records one decision per finding, and rejects the whole group if any item is stale, a model finding, or an unsure. There is no approve-all.
- After publication every decision request returns `lab-already-published`.

### 6.6 `/v2/labs/{labId}/publish`

`publishLab` has an empty body and evaluates in this order:

1. Route, session, CSRF.
2. Lab identifier, `Idempotency-Key`, and an empty body, otherwise `invalid-request`.
3. TA role, otherwise `ta-role-required`.
4. The lab exists and the caller is currently assigned to its class, otherwise `lab-not-found`.
5. A stored result for `(user, lab, key)` is returned exactly.
6. The lab is already published: `lab-already-published` with the original metadata.
7. `now < submissionDeadline`: `lab-still-open`.
8. An admission lease is outstanding, the roster is not frozen, judging or checking of a counted submission is not terminal or has failed, or sampling is not final: `lab-processing-incomplete` with counts.
9. A required decision is missing: `reviews-pending` with the count.
10. Under the lab lock, with access and generation verified again, one transaction writes the publication metadata, grade version 1 for every member of the frozen roster, the deductions, the audit event, the stored response, and the notification intents.
11. `200` with `labId`, `status`, `publishedAt`, and `publishedBy` only.

The results of steps 6 to 11 are stored for the key. Either every eligible grade becomes visible or none does. There is no unpublish and no per-student publication. Two different keys racing in different processes yield one `200` and one `lab-already-published`.

### 6.7 `/v2/labs/{labId}/grades`

- `formulaGrade = max(0, 10 - sum of confirmed deduction amounts)` for counted Accepted work, taking each violated constraint once whatever its evidence count. It is `0.00` for counted non-Accepted work and for no submission.
- `grade = clamp(formulaGrade + manualAdjustment.amount, 0, 10)`.
- `adjustStudentGrade` takes a `targetGrade` and a non-blank `reason`. The server stores `amount = targetGrade - formulaGrade`, the target, and the previous grade, and appends a grade version. A new adjustment replaces the current one. Publication metadata, the appeal window, and other students are unchanged (D08).
- Before publication a student receives `grade-not-found` and sees judge results only. After publication a student sees their own grade and deductions with constraint, amount, reason, and original evidence. Rejected proposals, unsure verdicts, sample membership, and review history are never part of a student projection.
- A `revision-conflict` for a grade names the student's id and the current `gradeVersion`.

### 6.8 `/v2/deductions/{deductionId}/appeals`

- A student may appeal each confirmed deduction of their own published submission once, while `publishedAt <= now < publishedAt + 7 days` (D03). `appealDeadline` is that end instant; an appeal at it is late. A correction does not move it.
- Judge verdicts and manual adjustments are not deductions and have no appeal.
- `createAppeal` evaluates: access to the deduction (`deduction-not-found`), the stored result for the key, an existing appeal (`appeal-already-exists`), the window (`appeal-window-closed`), then one transaction.
- `resolveAppeal` needs a non-blank `reason`. Acceptance marks the deduction `removed`, recomputes `formulaGrade`, keeps the stored adjustment amount, and appends a grade version, together with the appeal, the audit event, and the notification intent. Rejection changes no grade. A resolved appeal returns `appeal-already-resolved` and cannot be reopened; a TA corrects a later-found error with an adjustment.

### 6.9 `/v2/events`

- `GET /v2/events` is a `text/event-stream` authenticated by the session cookie. Each event has an `id`, an `event` name, and one `data` line holding an `EventData` object. The server sends a comment line at least every 25 seconds.
- An event names a resource and a revision. It never carries source, test data, evidence, a grade, or an identity. The client refetches the resource through its normal operation, which applies normal authorization.
- Events are filtered by the recipient's current membership before delivery. The server verifies access again at least every 60 seconds and closes the stream when the session or the access ends.
- Delivery is at least once. The client discards an `eventId` it has already processed. On reconnect it sends `Last-Event-ID`; when that id is unknown or too old, the first event is `stream.reset` and the client refetches everything it displays.
- Event types: `submission.updated`, `lab.updated`, `review.queue-changed`, `lab.readiness-changed`, `lab.published`, `grade.updated`, `appeal.updated`, `stream.reset`.

## 7. Decisions for contract review

### 7.1 Source reconciliation decisions

These come from [`docs/requirements/acceptance-matrix.md`](../../requirements/acceptance-matrix.md). Each row states the rule this draft proposes and the fixtures that fix its boundary. None is approved yet.

| ID  | Proposed rule                                                                                                                                                                                                          | Boundary fixtures                                                                                                                                                             | Awaiting                                                                                        |
| --- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| D01 | Binary units: source 65,536 bytes, output 1,048,576 bytes, memory 64 to 512 MiB with a 256 MiB default.                                                                                                                | Vectors `executionPolicy`, `sourceSize`, `outputSize`; scenarios `submissions.create.at-size-limit`, `.over-size-limit`, `.empty-body`                                        | Product-owner answer. A decimal answer changes four constants and their fixtures, nothing else. |
| D02 | Receipt is stamped when the complete body arrives. Accepted while `opensAt <= receivedAt < submissionDeadline`. Ties use `acceptanceSequence`. Publication passes its deadline check when `now >= submissionDeadline`. | Vectors `submissionWindow`, `publicationDeadline`, `countedSelection`; scenarios `submissions.create.*deadline*`, `publication.success-at-deadline`, `publication.still-open` | Team review.                                                                                    |
| D03 | Appeals are allowed while `publishedAt <= now < publishedAt + 7 days`. A correction does not restart the window. A same-key retry after the window returns the original appeal.                                        | Vector `appealWindow`; scenarios `appeals.create.just-before-window-end`, `.at-window-end`, `.window-not-restarted-by-correction`, `.replay-after-window`                     | Team review. A product-owner change needs a spec delta.                                         |
| D04 | Compilation has its own fixed limit of 10,000 ms, and exceeding it is `CE`.                                                                                                                                            | Vectors `executionPolicy.fixed`, `aggregateVerdict`; envelope fixture `judge-result-compile-timeout`                                                                          | Qualification of a backend that supports it (tasks 1.2 and 4.4).                                |
| D07 | The schedule is immutable after opening. Both enrollment paths and CSV exist. A class holds 100 students. A bundle is at most 10 MiB compressed, 100 MiB expanded, and 1,000 entries.                                  | Vectors `classCapacity`, `testBundleLimits`; scenarios `classes.members.add.*`, `classes.roster.import`, `labs.update.locked`, `labs.tests.upload.*`                          | Team review.                                                                                    |
| D08 | A manual score is stored as `targetGrade - formulaGrade` and is kept when an appeal removes a deduction; the grade is clamped to 0 through 10.                                                                         | Vector `grades`; scenarios `grades.adjust`, `appeals.resolve.accepted-with-adjustment`                                                                                        | Product and team review. Worked examples for task 8.5 are the `grades` vector cases.            |
| D09 | Evidence text matches the original line after trimming leading and trailing whitespace only.                                                                                                                           | Vector `evidence`; scenarios `reviews.decide.violation`, `reviews.decide.evidence-mismatch`                                                                                   | Resolved by the published scoring contract; listed for confirmation.                            |

### 7.2 Decisions this draft introduces

The design leaves these open or states them as proposals. A reviewer is asked to confirm or change each one.

1. **Cookie-only authentication.** `/v2` accepts the session cookie and no bearer header, so there is one authentication path and `EventSource` works without custom headers.
2. **Registration reveals an existing email.** Without email verification, `registration-rejected` distinguishes a used email. The draft limits this with the registration attempt limit. Login and (later) password reset never distinguish.
3. **Attempt limits and session lifetime.** The numbers in section 6.1 and the fixed seven-day session family are proposals.
4. **Two fractional digits for points**, in canonical string form in both directions.
5. **Whole-second CPU limits and whole-mebibyte memory limits**, expressed in milliseconds and bytes.
6. **Administrator account operations** `findUserByEmail` and `setUserRole` under `/v2/auth/users`. The design's family table does not list them, but role provisioning needs an operation.
7. **A roster entry needs an existing account.** There is no pending enrollment by email.
8. **Lock at opening** covers the schedule, the execution policy, and the test bundle; title and statement stay editable until publication.
9. **Stored idempotent results are the success and `409` results only**, for every keyed operation, always after current authorization.
10. **Publication state is evaluated before `expectedRevision`**, so an edit of a published lab reports `lab-already-published` rather than a revision conflict.
11. **Phase 9 operations are not part of 2.0** (section 10).

## 8. Frontend contract generation

`openapi.yaml` is the only source of the frontend's API types. Nothing in this section is installed by task 1.3.

- **Tool.** Proposed: `openapi-typescript`, which emits TypeScript declarations only and adds no runtime dependency. Its pinned version, license, and Node support are recorded and approved in the dependency inventory of task 1.2 before it is added.
- **Output.** `apps/web/lib/api/v2.d.ts`, committed, never edited by hand, and excluded from lint and format rules that would rewrite it.
- **Commands.** `npm run contract:generate` rewrites the file. `npm run contract:check` generates into memory and fails when the result differs from the committed file by one byte. Task 1.7 makes the check part of the documented root commands and a required CI result; task 2.1 delivers the first generated file.
- **Use.** Client boundaries type requests and responses through the generated `paths` and `components` types. Code branches on a problem's `type` URI, never on `title` or `detail`. Points stay strings until display; arithmetic uses integer hundredths.
- **Go backend.** Handlers are written by hand and verified against this contract by the scenario fixtures and schema validation of responses. No server code is generated.
- **Order of change.** A contract change is one reviewed pull request containing the schema, the fixtures, and this document. Regenerated types follow, then the implementation. A pull request that changes the schema without regenerating fails the drift check; one that changes generated types without the schema fails it too.

## 9. Fixtures and executable checks

```
fixtures/vectors.json            boundary tables: formats, limits, windows, sampling, grades, evidence
fixtures/scenarios/<family>.json one request and its exact response per scenario
```

A scenario has:

| Member               | Meaning                                                                                                                                                   |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`                 | Unique name, prefixed by the family.                                                                                                                      |
| `operationId`        | The operation in `openapi.yaml`, or `null` for a route that must not exist.                                                                               |
| `decision`, `covers` | The reconciliation decision and the specification requirements the scenario fixes.                                                                        |
| `given`              | `now` (the fixed server clock), `actor`, and `state` sentences to seed.                                                                                   |
| `request`            | `method`, `path`, `headers`, and one of `body` (JSON), `bodyText`, `bodyRepeat` (`text` repeated `count` times), or `bodyDescription` (a binary archive). |
| `requestSchemaValid` | `false` when the JSON body is meant to violate its request schema.                                                                                        |
| `response`           | `status`, `contentType`, listed `headers`, and `body` or `bodyText`.                                                                                      |
| `effects`            | Durable state and side effects, including their absence, that an implementation test must assert.                                                         |

Actors are `anonymous`, `admin`, `ta-assigned`, `ta-other-class`, `ta-removed`, `student-enrolled`, `student-other`, and `student-not-enrolled`. `<access:actor>` and `<refresh:...>` stand for credentials the test launcher issues; fixtures contain no real credential.

An implementation test seeds `given`, sends `request`, and compares `status`, the media type, every listed header, and every body member except a problem's `detail`. It then asserts `effects`. Task 1.6 builds the seeded, fixed-clock launcher these scenarios run against; until an implementation exists they are checked only for consistency.

`apps/api/test/contract/production-contract.test.ts` runs under `npm test` and verifies that:

- every operation is under `/v2`, uniquely named, and annotated, and its authentication, CSRF, role, idempotency, and revision problems follow from its declared actors and parameters;
- each operation's documented error statuses are exactly the statuses of its listed problems;
- the problem catalog, the table in section 5.1, and the nine types shared with v1.1 agree on status and title;
- request schemas are closed and cannot carry a role, a grade, or a deduction a caller must not choose;
- the boundary tables equal values derived again from the stated rules, and include a case just below, at, and just above each boundary;
- every scenario is a valid exchange of its operation, every operation and every problem type has a scenario, and the same key with the same actor and payload always shows the same response;
- envelope fixtures are accepted or rejected by the message schema, and both contracts share one verdict and limit vocabulary.

These checks prove the contract is internally consistent. They do not prove that any server behaves this way.

## 10. Operations reserved for enhancement phase 9

These operations belong to the listed families but are not specified in 2.0. Each needs a reviewed contract revision with fixtures before its implementation.

| Feature                                               | Family                | Task |
| ----------------------------------------------------- | --------------------- | ---- |
| Password reset by single-use email link               | `auth`                | 10.1 |
| Google sign-in and account linking                    | `auth`                | 10.1 |
| Constraint templates and lab cloning                  | `labs`                | 10.2 |
| Reference-solution validation before opening          | `labs`                | 10.3 |
| Test revision and rejudging after opening             | `labs`, `submissions` | 10.4 |
| Manual score for non-Accepted work before publication | `grades`              | 10.5 |
| Lab reports, class gradebook, and CSV export          | `grades`              | 10.6 |
| In-app notification list                              | `events`              | 10.7 |
| Evaluation-candidate intake                           | `reviews`             | 10.8 |

Phase 9 will relax two 2.0 rules: `lab-configuration-locked` for test bundles after opening (10.4) and `grades-not-published` for non-Accepted work (10.5).

## 11. Versions and evolution

- `info.version` in `openapi.yaml` is the contract version. `2.0.0` is assigned when the review in section 1 is recorded.
- Additive changes (a new operation, an optional request member, a response member, a problem type on a new operation) raise the minor version and need matching fixtures and a reviewed contract change.
- Changing authorization, concealment, evaluation order, idempotency scope, a stored-result rule, the meaning or representation of a member, or an existing problem's type, status, or title is breaking and needs a `/v3` route while `/v2` stays available until an announced date.
- The message envelopes are versioned independently of the HTTP contract.
- Contract and implementation changes are reviewed separately. A test, fixture, expected value, or contract is never edited merely to make an implementation pass.
