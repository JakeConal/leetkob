# PA#3 API contract — version 1.0

Revised November 20, 2026 to match the published PA#3 rubric. **Approved by the
owner on November 23, 2026** as the PA#3 implementation contract; it is published
as OpenAPI 3.1 in [`pa3-openapi.json`](pa3-openapi.json), with internal job and
checker schemas in [`schemas/`](schemas/) and exact problem bodies in
[`fixtures/pa3-problems.json`](fixtures/pa3-problems.json). The lecturer requires the
flow through product screens or API; seed scripts must not create the marked lab.
The existing full-product OpenSpec design remains canonical for later milestones.
This contract selects its PA#3 implementation boundary, not a replacement product.

Sources: [assignment](https://classroom.google.com/c/ODg2MTU1OTQ4NDc0/a/ODk5OTQ1NDY0NTgy/details),
[rubric](https://drive.google.com/file/d/1EGXZQNjbevJ5cE18rgXEujeKJ1hFeFvf/view),
[class brief](https://drive.google.com/file/d/1VK58AnV3_LWNWVX6jKxj1baPMDIA8QFm/view).

## Scope and access

- The Go production API uses `/v2`. The PA#2 publication contract, assertions and
  historical tag stay intact in a separate compatibility process. It cannot
  publish a production lab.
- Required accounts: one admin, two TAs in separate classes, and two students in
  the first TA's class. An operator command provisions only synthetic accounts,
  classes and memberships. It never creates the lab used for marking or inserts
  submissions/findings. Repeat provisioning does not silently reset credentials.
- Assigned TAs create labs, view class submissions, inspect findings and record
  decisions. Students read opened enrolled labs, submit and read only their own
  judge results. Admin is not an implicit TA. Current server-owned memberships
  decide access; caller role/class headers have no authority.
- Inaccessible IDs and another class's lab/queue return concealed 404. General
  wrong-role requests return 403. All protected responses use `Cache-Control:
no-store`. Lists are paginated using `limit` 1-100 (default 50) and an opaque
  cursor, and filtered by ownership before pagination.

Public registration, invitations/CSV import, refresh JWTs, Google sign-in,
constraint editing/rejudging, exports, grades, appeals and full production Publish
are later-milestone work. Existing umbrella requirements are not marked complete
by omitting these routes from PA#3. Model deductions are never published in PA#3.

## Sessions

| Method / route         | Request                        | Result                                                                              |
| ---------------------- | ------------------------------ | ----------------------------------------------------------------------------------- |
| `POST /v2/auth/login`  | `{email,password}`             | 200 `{user:{id,displayName,role},csrfToken,expiresAt}` and an opaque session cookie |
| `GET /v2/auth/me`      | Session cookie                 | 200 safe current user plus authorized class IDs/names                               |
| `POST /v2/auth/logout` | Session cookie and CSRF header | 204, revoking the session                                                           |
| `GET /v2/classes`      | Session cookie                 | 200 `{items:[{id,name}],nextCursor}` for current memberships                        |

Passwords are salted Argon2id hashes. Sessions expire after eight hours; store
only a digest of a cryptographically random token. Browser cookies are HttpOnly,
SameSite=Lax, Secure on deployed HTTPS and host-only. Local HTTP is explicitly
loopback-only. Next.js proxies browser API calls through the same origin without
exposing the cookie to JavaScript. API clients use the same cookie/CSRF contract.

Every authenticated mutation requires `X-CSRF-Token` matching the session. Reject
an Origin differing from the configured web origin; do not enable cross-origin
credentialed access. Login accepts JSON only, rejects an unexpected Origin, and
uses a generic authentication failure. Rate-limit login per IP (10/minute) and
account identifier (10/15 minutes) in durable state, with Retry-After on 429.
No token/password/hash is written to application logs or resource responses.

## Lab creation

`POST /v2/classes/{classId}/labs` requires an assigned TA, CSRF and
`Idempotency-Key`. The JSON request is:

```json
{
  "title": "Sorting lab",
  "statement": "Implement merge sort...",
  "opensAt": "2026-11-20T04:00:00Z",
  "deadline": "2026-11-25T15:00:00Z",
  "constraints": [
    { "id": "C1", "text": "Do not call `std::sort`.", "deduction": 3 },
    { "id": "C2", "text": "Implement merge sort.", "deduction": 4 }
  ],
  "tests": [{ "id": "01", "input": "3\n3 1 2\n", "expectedOutput": "1 2 3\n", "sample": true }],
  "limits": { "cpuSeconds": 1, "memoryMiB": 256 }
}
```

The example is a request-shape illustration; marking uses the actual published
`sort-merge` statement, tests and constraints supplied through this route.

Validation: UTF-8, title 1-200 characters, statement <=64 KiB, 1-20 unique
constraint IDs matching `[A-Za-z][A-Za-z0-9_-]{0,31}`, constraint text 1-4096
characters, deductions 0-10 with at most two decimal places, 1-500 unique tests
with nonempty IDs and a combined test payload <=10 MiB. No filesystem paths or
compiler command are accepted. Reject unknown fields and trailing JSON.
`opensAt < deadline` is required. CPU is 1-5 seconds, memory 64-512 MiB. Fixed
limits are C++17 `-O2`, compilation 10 seconds, per-test wall time 3*CPU, 16
processes including threads, 1 MiB output and 65,536 source bytes. Store monetary
and deduction quantities as fixed-precision integers internally.

Success is 201 `{id,classId,title,statement,opensAt,deadline,revision,
constraints,limits,modelNotice}`. The revision is 1. Source tests are stored
atomically with the lab. No partial lab is visible on failure. TA authoring must
work through screens/API; no manual database edits or lab-seeding workaround.

`GET /v2/classes/{classId}/labs` returns paginated safe labs.
`GET /v2/labs/{labId}` returns the safe lab and sample tests. Student reads expose
constraints/deductions, latest revision date and model-use notice, never hidden
tests or checker findings. Lab revision endpoints are not registered in this
slice, so active constraints/tests cannot be rewritten through an unchecked path.

## Submission acceptance and judging

`POST /v2/labs/{labId}/submissions` requires an enrolled student, CSRF and
`Idempotency-Key`; body `{source}` is bounded to 65,536 UTF-8 source bytes.
Success is 202 `{id,labId,state:"queued",receivedAt}` after durable commit only.
The full bounded request must arrive during `opensAt <= receivedAt < deadline`.
An admission lease/body deadline bounds incomplete requests. Source, submission,
counted selection, queue intent and replay acknowledgement commit together.

An idempotency key is 8-128 visible ASCII characters, scoped to actor/operation/
resource, with a request fingerprint. Same key/content replays the original
result even after the deadline; different content returns 409. Check current
ownership before replay. Retain completed keys at least 24 hours. Cache completed
business results, not transient 5xx. All mutation routes in this proposal use
these same key rules except login/logout.

A separate worker leases durable jobs. Expiry/restart permits retry; a monotonically
increasing attempt fence prevents late results from committing. Uniqueness by
submission and input revisions gives one stored verdict set even if execution
runs twice. Bounded infrastructure retries end in visible failure, not a student
WA/RE. Stop worker, submit, restart, and prove one stored result set in a test or
recording. Last valid receipt then server sequence selects counted work, even if
later WA replaces earlier AC. Superseded results cannot restore old findings.

Compile and execute only in qualified isolation. Prove all nine controls with
attack submissions on tag CI: no network; CPU; container-killing wall timeout;
memory including swap with allocation read-back; processes; output size;
non-root; no host mounts/socket; no environment/file/include-visible secrets.

`GET /v2/submissions/{id}` returns `{id,labId,state,receivedAt,judge}`;
`GET /v2/labs/{labId}/submissions` lists safe authorized submissions.
`judge` is absent until judged, then `{verdict,tests:[{id,verdict,timeMs}]}`.
Only the owner or assigned TA can read. Token comparison ignores whitespace.
Verdicts: AC, WA, TLE, CE, RE, MLE where distinguishable (otherwise documented
memory RE). No hidden input, expected/actual output, checker verdict, evidence,
provisional deduction or mark appears in student responses, per pinned FAQ-08.
The submission screen refetches current status until terminal; SSE is later work.

## Hard rules, semantic model and TA review

Current counted AC submissions run through hard rules first. Ban-list texts start
with `Do not use` or `Do not call`; backticks identify banned names. Analyze
syntax/resolved declarations, including namespace/using/macro traps. Comments,
strings, student-defined `sort`, and `<algorithm>` for `std::swap` do not trigger
false violations. Never call the model for a constraint hard rules decide.

Every semantic constraint calls the real configured model, subject to an atomic
lab cap (default USD 0.25). Send only statement, constraints and code stripped of
comments and known student identities with original-line mapping. Unsafe
sanitization produces unsure without dispatch. No labels, tests or student
metadata. Require per-constraint pass/violation/unsure and original-line evidence,
never a deduction or grade. Strict schema/evidence mismatch and timeout become
unsure; fake-model tests cover invalid output and timeout without network.
Reserve conservative spend before each call/retry; unknown usage remains charged.
Log sanitized prompts and provider request IDs, model, tokens, time and cost in a
restricted evidence artifact, not general application logs or student APIs.

`GET /v2/labs/{labId}/reviews` is assigned-TA-only and returns
`{items:[finding],nextCursor,processingPending,reviewPending}`. Every finding
includes submission/constraint IDs/revisions, original source (TA only), checker
verdict/source, evidence `{line,text}`, unsure cause, TA-configured deduction,
required-review flag and separate optional TA decision. All violations/unsure and
a stable random ceil(10%) of pass pairs need review; queue reads never redraw it.

`POST /v2/labs/{labId}/reviews/{findingId}/decision` requires assigned TA, CSRF,
idempotency key and `{expectedRevision,verdict,reason,evidence}`. Verdict is pass
or violation, reason is nonempty and <=2000 characters. Violation needs valid
original-line evidence. Success 200 returns the finding with a separate decision,
new revision and audit actor/time. Stale revision returns 409. Unsure has no default;
checker provenance is immutable. No action publishes marks in this checkpoint.
The compatibility publication process uses fixture-only storage, never production.

## Errors and internal messages

RFC 9457 JSON errors use the existing `https://leetkob.example/problems/` namespace:
400 invalid-request, 401 authentication-required, 403 role-required,
404 resource-not-found, 409 conflict, 413 payload-too-large, 429 rate-limited,
503 unavailable, and fixed generic 500 internal-error. `instance` is the request
path without query. Never expose cross-class details, exception text or secrets.
Use specific conflict extensions for closed submission window or stale revision;
contract fixtures define exact bodies before implementation.

Persisted job envelope:
`{version:1,jobId,attempt,kind,submissionId,labId,generation,testRevision,
constraintRevision,sourceSha256}`. Results echo revision tuple and fence; stale
results cannot update current work. This is not a public HTTP route and carries
no credentials.

## Checker and evidence delivery

`checker --data <dataset-dir> --predictions <output.jsonl>` discovers unseen
problem IDs generically and reads only statements, constraints and solutions.
Each JSONL line is `{solution,constraint,verdict,evidence:[{line,text}]}`.
`MODEL_OFF=1` opens no network, decides every ban-list pair and writes unsure for
all semantic pairs. The Docker image includes its parser; the marker must not
need a Docker socket inside it. Run `eval.mjs` separately, unmodified. For PA#3,
scorer exit 0 or 1 is acceptable; exit 2 is invalid. No PA#3 recall/unsure pass
threshold is claimed. Zero wrongful hard-rule deductions still matters under
criterion 3. Keep dev/scorer bytes pinned; add >=5 labelled/tagged team cases in
a separate folder, including trap and injection cases.

Commit the real-model dev predictions, scorer report and call log with provider
request IDs. Record model and arithmetic for 80 submissions in README/AI-LOG.
No unit/contract test calls a real provider. A paid evaluation and deployment
remain separately authorized operations. PA3.md ultimately links URL, five test
accounts, tag CI, checker/Docker/model-off commands, committed run, and <=5-minute
fallback recording. Keep the deployment up until PA#4 marks are released.
