# Spec Delta

## ADDED Requirements

### Requirement: Versioned production publication boundary
The system SHALL expose `POST /v2/labs/{labId}/publish` for production labs with a UUID lab ID, required Idempotency-Key, and empty body. The unversioned PA#2 operation SHALL retain its v1.1 contract in compatibility mode and SHALL NOT provide an alternate path to publish production-managed labs.

#### Scenario: PA#2 contract is exercised
- **WHEN** the existing unversioned contract suite runs against its compatibility fixtures
- **THEN** documented request validation, evaluation order, statuses, privacy, and response replay remain unchanged

#### Scenario: Legacy operation targets production data
- **WHEN** a caller uses the unversioned route in the production deployment
- **THEN** the route is not exposed there and cannot bypass production readiness or authorization

### Requirement: Current authorization before production replay
The production operation SHALL authenticate the current session and authorize current TA role and class ownership before reading an idempotent response. Missing identity SHALL yield 401, non-TA identity 403, and absent or inaccessible labs a concealed 404. Revoked permissions SHALL prevent replay of protected results.

#### Scenario: Former TA replays a publication key
- **WHEN** a TA loses class ownership and retries a previously successful production request
- **THEN** the request receives concealed not-found rather than the cached publication metadata

### Requirement: Complete processing before production publication
Production publication SHALL require the submission deadline to have passed, counted submissions to be finalized, all current judge and checker work to be terminal, pass sampling to be finalized, and every required TA review to be resolved. Processing failures or missing expected findings SHALL NOT be treated as an empty completed queue.

#### Scenario: Deadline passes while judging continues
- **WHEN** the deadline has passed and the visible review queue is empty but a counted submission still needs judging or checking
- **THEN** publication returns 409 with the documented lab-processing-incomplete problem and makes no grade visible

#### Scenario: Required reviews remain
- **WHEN** processing is complete but a violation, unsure, or sampled pass lacks a TA decision
- **THEN** publication returns 409 reviews-pending with the unresolved item count

#### Scenario: Empty or non-Accepted lab is ready
- **WHEN** the deadline has passed, every roster member's counted-submission state is finalized, and no Accepted findings require review
- **THEN** students without a submission and those with non-Accepted work receive their applicable zero or manual grade and publication can proceed

### Requirement: Durable atomic grade visibility
Production publication SHALL atomically record immutable publication actor/time, the finalized grade snapshot for the whole lab, an audit event, and the replay response. It SHALL use a consistent current revision of submissions, constraints, findings, and decisions across processes. Either all eligible student grades become visible or none do.

#### Scenario: Storage fails during publication
- **WHEN** any part of the transaction fails before commit
- **THEN** no partial grades, publication metadata, completed replay, or notification intent become visible

#### Scenario: Two API processes publish concurrently
- **WHEN** two authorized requests with different keys target the same ready lab
- **THEN** one publication commits and the other receives 409 lab-already-published with the original safe metadata

### Requirement: Production retry and irreversible publication
Production idempotency SHALL be scoped to subject, lab, and key and retain completed responses for at least 24 hours. After current authorization, same-key retries SHALL replay the original completed result; new keys after publication SHALL return 409 lab-already-published. Whole-lab unpublish and per-student publication SHALL be unavailable.

#### Scenario: Readiness rejection is retried
- **WHEN** a caller retries the same key after the original deterministic 409 condition has been resolved
- **THEN** the original response is replayed and a deliberate new attempt requires a new key

#### Scenario: Student grade is corrected later
- **WHEN** an individual grade correction or accepted appeal commits
- **THEN** original publication metadata and stored publication responses remain unchanged

### Requirement: Safe production publication responses
Successful production responses SHALL contain only labId, status, publishedAt, and publishedBy. Errors SHALL use documented absolute RFC 9457 problem types and safe counts or timestamps, with no student identities, source, evidence, credentials, or cross-class details.

#### Scenario: Processing prevents publication
- **WHEN** the operation reports incomplete work
- **THEN** its problem response exposes readiness counts only and no per-student processing records
