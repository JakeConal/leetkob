# LeeTKOB publish-lab API — contract, version 1

CSC13114 · PA#2 · frozen 24 Oct 2026

This file is the specification. `test/contract/publish-lab.contract.test.ts` checks an implementation against it. If the code and this file disagree, the code is wrong. Change this contract only in a separate commit or pull request that explains why.

## 1. Conventions

- The endpoint is `POST /labs/{labId}/publish`; there is no trailing slash or query string.
- `{labId}` is a canonical UUID string. UUID versions 1 through 5 are accepted, case-insensitively.
- Requests carry `Authorization: Bearer <token>`. The token represents one subject, one role, assigned class IDs, and an expiry. Clients treat it as opaque.
- Requests carry `Idempotency-Key`, 8 to 128 visible ASCII characters (`!` through `~`). Whitespace, control characters, and duplicate header values are invalid.
- The request body is empty. `{}`, `null`, and whitespace are bodies and are invalid.
- Successful and problem responses are UTF-8 JSON and carry `Cache-Control: no-store`.
- Timestamps are RFC 3339 strings in UTC with milliseconds, for example `2026-10-24T01:02:03.000Z`.
- Unknown response members are ignored by clients. There are no optional request members.

## 2. Errors

Every error is a problem-details object (RFC 9457) with `Content-Type: application/problem+json`:

```http
HTTP/1.1 409 Conflict
Content-Type: application/problem+json; charset=utf-8
Cache-Control: no-store

{
  "type": "https://leetkob.example/problems/reviews-pending",
  "title": "Reviews pending",
  "status": 409,
  "detail": "All required reviews must be resolved before marks can be published.",
  "instance": "/labs/f6729f3d-d07e-497d-83fb-86a85810d84d/publish",
  "pendingReviews": 2
}
```

| Member                                | Rule                                                                                                                                                     |
| ------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `type`                                | Absolute URI from the problem table below; clients branch on this value.                                                                                 |
| `title`                               | Stable title from the table below.                                                                                                                       |
| `status`                              | Number equal to the HTTP response status.                                                                                                                |
| `detail`                              | Safe sentence about this occurrence; never contains source code, student identity, secrets, stack traces, database details, or another class identifier. |
| `instance`                            | The request path, without a query string.                                                                                                                |
| `pendingReviews`                      | Only on `reviews-pending`; non-negative integer count at the first completed attempt.                                                                    |
| `labId`, `publishedAt`, `publishedBy` | Only on `lab-already-published`; original publication metadata.                                                                                          |

| Type suffix               | Status | Title                   | When                                                                                                                      |
| ------------------------- | -----: | ----------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `invalid-request`         |    400 | Invalid request         | Invalid UUID, missing/invalid/duplicate idempotency key, or any request body.                                             |
| `authentication-required` |    401 | Authentication required | Missing, malformed, tampered, structurally invalid, or expired bearer token.                                              |
| `ta-role-required`        |    403 | TA role required        | Authenticated subject does not have role `ta`.                                                                            |
| `lab-not-found`           |    404 | Lab not found           | The lab does not exist **or** belongs to a class not assigned to this TA. These cases are deliberately indistinguishable. |
| `route-not-found`         |    404 | Route not found         | Wrong method, path, trailing slash, or any query string.                                                                  |
| `reviews-pending`         |    409 | Reviews pending         | One or more verdicts still require TA review.                                                                             |
| `lab-already-published`   |    409 | Lab already published   | The lab was published previously and this request uses a different idempotency key.                                       |
| `internal-error`          |    500 | Internal server error   | Unexpected failure. `detail` is always the fixed generic sentence shown by the implementation.                            |

No response exposes whether a concealed cross-class lab exists. All failures leave the lab state unchanged.

## 3. Resources

**Caller identity**

The verified bearer claim supplies:

```json
{
  "subject": "ta-1",
  "role": "ta",
  "classIds": ["class-a"]
}
```

- `subject` is a non-empty opaque user identifier.
- `role` is `admin`, `ta`, or `student`; only `ta` may call this endpoint.
- `classIds` is the set of classes assigned to the TA.
- A lab is visible to the caller only when its `classId` occurs in this set.

**Publication**

```json
{
  "labId": "0d0f366c-f9cf-45e8-bbf2-ab1b8e370ec8",
  "status": "published",
  "publishedAt": "2026-10-24T01:02:03.000Z",
  "publishedBy": "ta-1"
}
```

| Field         | Rule                                                                        |
| ------------- | --------------------------------------------------------------------------- |
| `labId`       | Exact UUID from the request.                                                |
| `status`      | Literal string `published`.                                                 |
| `publishedAt` | Server-selected publication time; immutable after success.                  |
| `publishedBy` | Verified subject that performed the first successful transition; immutable. |

The response never contains submissions, student names or IDs, source code, review evidence, deductions, or credentials.

## 4. Endpoint

### 4.1 `POST /labs/{labId}/publish`

Publishes marks for one lab. This is allowed only after every required review is resolved.

Request:

```http
POST /labs/0d0f366c-f9cf-45e8-bbf2-ab1b8e370ec8/publish HTTP/1.1
Authorization: Bearer <opaque-signed-token>
Idempotency-Key: publish-request-0001
Content-Length: 0
```

Success:

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store

{
  "labId": "0d0f366c-f9cf-45e8-bbf2-ab1b8e370ec8",
  "status": "published",
  "publishedAt": "2026-10-24T01:02:03.000Z",
  "publishedBy": "ta-1"
}
```

The server evaluates the request in this order and stops at the first completed result:

1. Exact method, path, and absence of query string; otherwise `404 route-not-found`.
2. Bearer verification and expiry; otherwise `401 authentication-required`.
3. UUID, idempotency key, and empty body rules; otherwise `400 invalid-request`.
4. A completed result for `(subject, labId, idempotency key)`; replay it exactly.
5. Caller role `ta`; otherwise `403 ta-role-required`.
6. Lab existence and class ownership; otherwise `404 lab-not-found`.
7. Lab is not already published; otherwise `409 lab-already-published` with original metadata.
8. Pending-review count is zero; otherwise `409 reviews-pending` with the count.
9. In one atomic transition, set status to `published`, record the verified subject and current timestamp, store the idempotent response, and return `200` Publication.

**Authorization and ownership.** “Authenticated” alone is insufficient. The caller must be a TA and the lab's class must be in that TA's signed `classIds`. A TA from another class receives the same `404 lab-not-found` body shape as an absent lab.

**State change.** A successful transition changes only publication state, `publishedAt`, and `publishedBy`. It does not alter verdicts, review evidence, deductions, submissions, or identities. Any failure changes nothing.

**Idempotency.** A timeout or dropped connection is retried with the same key.

- Scope is the exact tuple `(verified subject, labId, Idempotency-Key)`.
- The first completed domain result (`200`, `403`, `404`, or `409`) is stored with its status, content type, and body.
- Same scope again returns that exact stored result. Mutable state is not re-evaluated and no second transition occurs.
- Authentication, routing, or input failures (`400`, `401`, route `404`) are not stored because no valid actor/lab/key scope exists.
- A different key after success returns `409 lab-already-published` with the original metadata.
- Two different keys arriving concurrently are serialized per lab: exactly one can return `200`; the other returns `409 lab-already-published`.
- A completed key is retained for at least 24 hours when durable storage replaces the PA#2 in-memory adapter.

## 5. Versions and evolution

- Version 1 is the behavior frozen in this file even though the course-specified route is unprefixed.
- Additive response members and new problem types may be introduced only with matching contract tests and a documented contract change.
- Changing authorization, ownership concealment, field meaning, idempotency scope, check order, state transition, or an existing problem type/status is breaking and requires a new endpoint version while this behavior remains available through its announced sunset date.
- Contract changes and implementation changes are reviewed separately when practical. Tests, eval data, or expected labels are never edited merely to make an implementation pass.
