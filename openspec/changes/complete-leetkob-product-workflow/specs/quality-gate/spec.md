# Spec Delta

## ADDED Requirements

### Requirement: Worker and integration check coverage
The documented root quality commands SHALL cover the API, web, and all added worker/checker packages. Deterministic tests SHALL include authorization isolation, deadline and revision races, job recovery, evidence validation, budget reservation, atomic publication, and appeal consistency without real model requests.

#### Scenario: A worker package is added
- **WHEN** a contributor runs the existing root lint, format, typecheck, and test commands after documented setup
- **THEN** the worker participates in the corresponding checks and cannot silently escape CI coverage

### Requirement: Contract-first production evolution
New public endpoints and message contracts SHALL have reviewed request, response, error, authorization, retry, and revision semantics plus executable contract tests before their implementation changes. The existing PA#2 publication suite SHALL remain intact; changed production behavior SHALL use the documented versioned route.

#### Scenario: Production publication adds a readiness error
- **WHEN** processing-incomplete behavior is introduced
- **THEN** it is documented and contract-tested for the production version while unversioned PA#2 fixtures retain their established behavior

### Requirement: Checker evaluation merge gate
Changes affecting prompts, deterministic rules, sanitization, model configuration, or checker behavior SHALL require passing evaluation evidence for their relevant revision on both dev and team datasets. Missing, stale, invalid, or failing evidence SHALL block merging once the approved evaluation check is enabled, alongside existing required checks.

#### Scenario: Pull request produces a wrongful deduction
- **WHEN** evaluation predicts violation for any pass-labelled pair
- **THEN** the evaluation result fails and the protected merge gate rejects that revision

#### Scenario: Evaluation is skipped on an affected revision
- **WHEN** a checker-affecting change has no valid approved evidence
- **THEN** the gate reports pending or failure and cannot turn green through an unconditional skip

### Requirement: Auditable milestone evidence
The repository SHALL preserve PA#2 evidence and add linked PA#3-PA#5 evidence for the complete flow, evaluation rejection and repair, sandbox attack results, red-team findings, load measurements, deployment, and post-mortem. Reported completion SHALL be supported by observed results for the documented revision.

#### Scenario: Lecturer audits the expanded workflow
- **WHEN** the reviewer follows the milestone evidence index
- **THEN** each claimed result identifies its source revision and reproducible command or specific recorded artifact, and unperformed checks are labelled honestly
