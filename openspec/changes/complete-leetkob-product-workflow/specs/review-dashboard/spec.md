# Spec Delta

## MODIFIED Requirements

### Requirement: Publication gate presentation
The dashboard SHALL present the submission deadline, processing readiness, and unresolved review count from authoritative lab state. It SHALL show publication blocked and disable Publish while the deadline is open, processing or sampling is incomplete, required decisions remain, or readiness is unavailable. A published lab SHALL show its immutable publication metadata.

#### Scenario: Pending reviews are shown
- **WHEN** the current lab has one or more pending reviews
- **THEN** the page shows `Publication blocked`, explains that every review must be resolved, reports the count, and disables `Publish lab`

#### Scenario: Submission deadline status is shown
- **WHEN** the current lab is displayed
- **THEN** the page exposes whether the submission deadline has passed as a distinct publication check without exposing protected data to an unauthorized viewer

#### Scenario: Processing is incomplete
- **WHEN** the displayed pending review count is zero but current judging, checking, or sampling has not finished
- **THEN** the page explains the remaining processing and keeps Publish disabled

### Requirement: Safe dashboard data
Demonstration mode SHALL use labelled synthetic rows and SHALL NOT request or render real student identity, source code, credentials, or evidence. Live mode SHALL require an assigned TA and display only authorized class data. Credentials SHALL never be rendered in either mode, and protected responses SHALL not enter shared public caches.

#### Scenario: Reviewer inspects page content
- **WHEN** the demonstration dashboard is rendered in any query state
- **THEN** no protected student data, source code, credentials, or cross-class identifiers are present

#### Scenario: Authorized TA opens live review
- **WHEN** an assigned TA opens a lab review queue
- **THEN** the page loads current findings and original source through authorized requests and clearly differs from demonstration mode

## ADDED Requirements

### Requirement: Live review interactions
The live dashboard SHALL support lab selection, source/constraint grouping, individual evidence inspection, explicit unsure resolution, allowed deterministic group approval, and conflict-aware decisions. Model violations SHALL have no approve-all control, and stale decisions SHALL prompt refresh rather than silently overwrite changes.

#### Scenario: Review is superseded while open
- **WHEN** a submission or constraint revision replaces the finding the TA is viewing
- **THEN** the interface identifies the stale view and requires refreshed evidence before another decision

### Requirement: Deliberate publication action
The production Publish control SHALL invoke the versioned production operation, retain its key for transport retries, and require a fresh deliberate action after a completed readiness rejection. A conflict indicating prior publication SHALL display the existing publication rather than attempting another grade release.

#### Scenario: Publish response is lost
- **WHEN** the browser retries an uncertain publication request
- **THEN** it reuses the same key and displays the server's committed result without a duplicate operation

### Requirement: Accessible operational workflow
Live authoring, submission, review, grade, and appeal screens SHALL follow the repository theme, support keyboard navigation and visible focus, provide readable non-color status cues, and preserve reading order on narrow screens. Long source code SHALL scroll within its own region without overflowing the page.

#### Scenario: TA reviews on a narrow viewport
- **WHEN** a two-column evidence layout cannot fit
- **THEN** controls and evidence stack in reading order and every decision remains keyboard accessible

### Requirement: Authenticated client state lifecycle
Client queries and realtime subscriptions SHALL be scoped to the current user and class, refresh after committed changes, and clear protected cached data on logout or access loss. Failed refreshes SHALL retain an explicit stale or unavailable state rather than implying readiness.

#### Scenario: User changes account
- **WHEN** a TA logs out and another user signs in on the same browser
- **THEN** no previous user's source, grade, or review data is displayed or replayed from cache
