# Spec Delta

## Purpose

Defines account access and class ownership so each participant can perform their role without accessing another class or student's protected work.

## ADDED Requirements

### Requirement: Account and session lifecycle
The system SHALL support email/password registration and login, short-lived signed access tokens, rotating refresh sessions, and logout. Registration SHALL create only a student account; passwords and refresh tokens SHALL NOT be stored in plaintext or returned by resource APIs.

#### Scenario: Student registers and signs in
- **WHEN** a person supplies a valid unused email and an acceptable password
- **THEN** the system creates a student account and allows authenticated access without permitting caller-selected elevated roles

#### Scenario: Session is revoked or refresh token reused
- **WHEN** a revoked session or a previously consumed refresh token is presented
- **THEN** access or refresh fails and refresh-token reuse revokes the affected session family

### Requirement: Verified and current authorization
Protected operations SHALL derive identity from verified authentication and current server-owned roles and memberships. Caller-supplied role or class headers SHALL NOT grant access. Only an administrator SHALL provision elevated roles.

#### Scenario: Class assignment is removed
- **WHEN** a TA loses an assignment while retaining an unexpired access token
- **THEN** subsequent protected reads, mutations, and event delivery for that class are denied

#### Scenario: Caller spoofs a role
- **WHEN** a student sends a TA role header or edits an unsigned identity claim
- **THEN** the system grants no TA permissions

### Requirement: Class administration and enrollment
Administrators SHALL create classes and assign multiple TAs. Students and TAs SHALL be able to belong to multiple classes. The system SHALL support student enrollment by invitation code and by assigned-TA roster entry, including validated CSV import, with a maximum of 100 students per class.

#### Scenario: Student joins a class twice
- **WHEN** an authenticated student redeems a valid invitation for a class they already joined
- **THEN** the class contains one membership and no duplicate enrollment is created

#### Scenario: TA imports a roster
- **WHEN** an assigned TA submits CSV rows containing valid, duplicate, invalid, or over-capacity entries
- **THEN** the system reports each rejected row, prevents duplicate memberships, and never exceeds 100 enrolled students

### Requirement: Resource and event isolation
TAs SHALL access only labs, submissions, queues, and grades in assigned classes. Students SHALL access only their enrolled classes' available labs and their own submissions, grades, and appeals. These rules SHALL cover direct identifiers, lists, exports, file downloads, and realtime events.

#### Scenario: Student requests another student's submission
- **WHEN** a student guesses a submission identifier belonging to a classmate
- **THEN** the system returns a concealed not-found response and discloses neither source nor existence

#### Scenario: TA requests another class
- **WHEN** a TA requests another class's lab, source download, review queue, export, or event stream
- **THEN** the system denies access without revealing protected data

### Requirement: Recoverable and federated account access
The enhancement phase SHALL provide expiring single-use password reset links and Google sign-in using basic identity scopes. Account linking SHALL require proof of control of the existing account; a matching email alone SHALL NOT link an untrusted identity.

#### Scenario: Password reset is requested
- **WHEN** someone submits an email to the reset flow
- **THEN** the public response does not reveal whether the account exists, and any issued token expires, is single use, and revokes prior sessions when consumed

#### Scenario: Google identity is linked
- **WHEN** a signed-in user proves control of a Google identity through a validated sign-in flow
- **THEN** the system links that identity using basic scopes without requesting classroom or mailbox permissions

### Requirement: Authentication abuse controls
The system SHALL rate-limit authentication, invitation redemption, and password-reset attempts and protect browser mutations against cross-site request forgery. Credentials SHALL NOT appear in logs, URLs, or client-readable persistent token storage.

#### Scenario: Repeated failed authentication
- **WHEN** an actor exceeds the documented authentication attempt limit
- **THEN** further attempts receive a bounded retry response without revealing account existence
