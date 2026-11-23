# identity-and-classes Specification

## Purpose

Provide authenticated access for the synthetic accounts used to demonstrate and assess the PA#3 lab workflow without exposing another class's work.

## Requirements

### Requirement: Synthetic assessment account access

The system SHALL support sign-in and logout for one admin, two TAs assigned to different classes, and two students in the first TA's class. Provisioning SHALL create only synthetic accounts, classes and memberships, never the marked lab or its submissions. Passwords and session credentials SHALL NOT be returned by resource reads or logged.

#### Scenario: PA3-ID-01 Assessor signs in

- **WHEN** the assessor signs in using each account documented in PA3.md
- **THEN** the correct current role and class access is available and logout invalidates the session

#### Scenario: PA3-ID-02 Provisioning is repeated

- **WHEN** the setup command is run against an already initialized database
- **THEN** it does not create duplicates, reset existing credentials silently or insert the lab used for marking

### Requirement: Current role and class isolation

Protected operations SHALL check verified identity and current server-owned role and membership. Caller-supplied role/class headers SHALL NOT grant access. Another class's lab, source, submission and queue SHALL use the PA#2 contract's concealed-not-found behavior. Students SHALL access only their own submissions and results.

#### Scenario: PA3-ID-03 Another TA guesses a lab ID

- **WHEN** the second TA requests the first TA's lab, source or review queue
- **THEN** the operation returns a concealed 404 without revealing protected data

#### Scenario: PA3-ID-04 Student spoofs access

- **WHEN** a student sends TA headers or requests another student's submission
- **THEN** no TA permission is granted and the other submission is not disclosed

#### Scenario: PA3-ID-05 Membership is removed

- **WHEN** a session remains unexpired after its user's class access is removed
- **THEN** subsequent protected requests cannot access that class

### Requirement: Protected browser sessions

Authenticated browser state SHALL remain outside client-readable persistent token storage. Mutations SHALL reject cross-site request forgery. Authentication failures SHALL be generic and rate-limited, and session expiry/revocation SHALL deny subsequent access.

#### Scenario: PA3-ID-06 Cross-origin mutation

- **WHEN** a signed-in browser receives an attempted mutation from a different origin or without its session-bound CSRF proof
- **THEN** the mutation is rejected without changing protected state
