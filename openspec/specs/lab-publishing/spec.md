# lab-publishing Specification

## Purpose

Defines the externally observable authorization, safety, idempotency, state-transition, and error behavior for publishing a LeeTKOB lab.

## Requirements

### Requirement: Publish endpoint identity
The system SHALL expose `POST /labs/{labId}/publish` for the publication operation, accept a UUID lab identifier, require an `Idempotency-Key` header, and accept no request body.

#### Scenario: Valid request shape
- **WHEN** a caller sends the endpoint a UUID lab identifier, a valid idempotency key, and an empty body
- **THEN** the system evaluates the publication request

#### Scenario: Invalid request shape
- **WHEN** the lab identifier or idempotency key is invalid, the header is missing, or the request body is non-empty
- **THEN** the system returns status `400` with the `invalid-request` problem type and makes no state change

### Requirement: TA authentication and role
The system SHALL allow only an authenticated user with the `ta` role to attempt publication.

#### Scenario: Missing authentication
- **WHEN** the request has no valid authenticated identity
- **THEN** the system returns status `401` with the `authentication-required` problem type

#### Scenario: Non-TA identity
- **WHEN** an authenticated student or administrator attempts publication
- **THEN** the system returns status `403` with the `ta-role-required` problem type

### Requirement: Class ownership boundary
The system SHALL let a TA publish only a lab belonging to a class assigned to that TA and SHALL conceal labs outside that boundary.

#### Scenario: TA owns the lab class
- **WHEN** a TA assigned to the lab's class sends a valid publication request
- **THEN** the system continues evaluating publication preconditions

#### Scenario: Lab is absent or belongs to another class
- **WHEN** the lab does not exist or the TA is not assigned to its class
- **THEN** the system returns status `404` with the `lab-not-found` problem type

### Requirement: Pending-review publication gate
The system SHALL refuse publication while any submission verdict for the lab still requires TA review.

#### Scenario: Reviews are pending
- **WHEN** at least one lab review is pending
- **THEN** the system returns status `409` with the `reviews-pending` problem type, reports the pending count, and leaves the lab unpublished

#### Scenario: All reviews are resolved
- **WHEN** no lab reviews are pending
- **THEN** the system permits the publication transition

### Requirement: Submission-deadline publication gate
The system SHALL refuse whole-lab publication before the lab's submission deadline and SHALL use the server clock rather than a client-supplied time.

#### Scenario: Submission period is still open
- **WHEN** the server time is before the lab's submission deadline
- **THEN** the system returns status `409` with the `lab-still-open` problem type, reports the deadline, and leaves the lab unpublished

#### Scenario: Submission deadline has passed
- **WHEN** the server time is equal to or later than the lab's submission deadline
- **THEN** the system continues evaluating the pending-review gate

#### Scenario: Deadline rejection is retried
- **WHEN** a deadline rejection is retried with the same caller, lab, and idempotency key after the deadline passes
- **THEN** the system returns the exact original rejection without re-evaluating the server time

### Requirement: Atomic publication transition
The system SHALL atomically move an eligible lab from `ready` to `published` and record the publishing TA and an RFC 3339 UTC timestamp.

#### Scenario: First successful publication
- **WHEN** an authorized TA publishes an eligible ready lab with a new idempotency key
- **THEN** the system returns status `200` with `labId`, `status: published`, `publishedAt`, and `publishedBy`, and persists those values as one state change

#### Scenario: State cannot be partially changed
- **WHEN** publication cannot complete
- **THEN** the lab remains in its previous state with no publication actor or timestamp recorded

### Requirement: Idempotent retries
The system SHALL bind the first completed request for a lab and idempotency key to its exact status, response body, and relevant response headers.

#### Scenario: Retry after success
- **WHEN** the same authorized caller retries the same lab publication with the same idempotency key
- **THEN** the system returns the original successful representation without a second state transition

#### Scenario: Retry after deterministic rejection
- **WHEN** the same request is retried with an idempotency key whose completed response was a deterministic `4xx` rejection
- **THEN** the system returns the original rejection representation without re-evaluating mutable preconditions

#### Scenario: Different key after publication
- **WHEN** an already published lab receives a publication request with a different idempotency key
- **THEN** the system returns status `409` with the `lab-already-published` problem type and the original publication metadata

### Requirement: RFC 9457 problem details
The system SHALL represent every error as `application/problem+json` with absolute `type`, non-empty `title`, numeric `status`, explanatory `detail`, and request-specific `instance` fields.

#### Scenario: Documented error response
- **WHEN** any publication error occurs
- **THEN** the response body conforms to RFC 9457 and uses exactly one documented problem type for its status

### Requirement: Publication response privacy
The system SHALL expose only lab publication metadata and SHALL NOT include student identities, source code, model credentials, or review evidence in publication responses or problem details.

#### Scenario: Successful response contains safe fields
- **WHEN** publication succeeds
- **THEN** the response includes only the documented publication fields

#### Scenario: Error response does not disclose protected data
- **WHEN** publication fails
- **THEN** the problem details contain no student data, source code, secrets, or cross-class identifiers
