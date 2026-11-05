# Spec Delta

## Purpose

Defines evidence-producing checks of accepted C++ work without delegating grades to a model or exposing student identity and execution credentials.

## ADDED Requirements

### Requirement: Eligible versioned checking
Only Accepted submissions SHALL be checked for constraints. Active findings SHALL correspond to the counted submission, active test revision, and active constraint revision. Early checking is permitted, but historical findings SHALL NOT enter the current review queue.

#### Scenario: Counted submission is not Accepted
- **WHEN** judging produces WA, TLE, MLE, RE, or CE for the counted submission
- **THEN** no constraint checking is requested for that submission

#### Scenario: A stale checker result arrives
- **WHEN** a newer submission, test revision, or constraint revision has superseded the input
- **THEN** the result is historical only and cannot satisfy current review or readiness

### Requirement: Sanitized model input
The model SHALL receive only statement, constraints, and code with comments, personal names, and student IDs removed or anonymized, retaining a mapping to original source lines. Code SHALL be treated as untrusted data. The model SHALL have no tools and SHALL receive no test contents, account metadata, or credentials.

#### Scenario: Comment or literal contains injected instructions
- **WHEN** submitted code includes text asking the checker to ignore constraints or reveal instructions
- **THEN** comments are removed, remaining code is isolated as data, and output is subject to the same schema and evidence checks

#### Scenario: Anonymization cannot preserve trustworthy mapping
- **WHEN** sanitization cannot safely remove identifying material while preserving analysis and line correspondence
- **THEN** the system avoids sending that input and records unsure with a preparation-failure reason for TA review

### Requirement: Syntax-aware ban-list rules
Constraints starting with `Do not use` or `Do not call` SHALL be evaluated against the banned names enclosed in backticks using syntax-aware deterministic checks. Rules SHALL distinguish actual forbidden constructs from comments, strings, unrelated includes, and student-defined functions. Resolved rules SHALL not incur a model call.

#### Scenario: User-defined sort is allowed
- **WHEN** code calls its own sort function and only mentions a banned library sort in a comment
- **THEN** the ban-list finding is pass rather than a textual-name false positive

#### Scenario: Forbidden call uses indirection
- **WHEN** a banned construct is reached through a supported alias, using declaration, macro, or wrapper
- **THEN** the rule reports violation with original code evidence; unsupported or ambiguous resolution yields unsure rather than an invented pass or violation

### Requirement: Semantic verdict schema
Semantic constraints SHALL use a fixed model step returning pass, violation, or unsure per constraint, with reason and evidence entries containing original line number and text. The model SHALL NOT supply grades or deduction amounts. Findings SHALL record checker, prompt, and model versions and source kind.

#### Scenario: Model supplies an unauthorized field or constraint
- **WHEN** output includes a grade, unknown constraint, duplicate constraint, missing required result, or invalid verdict
- **THEN** validation rejects the malformed output rather than accepting an implied grading decision

### Requirement: Evidence and failure handling
Violation evidence SHALL contain at least one valid original line whose text matches after trimming leading and trailing whitespace only. Invalid schema SHALL be retried at most once; repeated schema failure, bad evidence, timeout, provider failure, or budget exhaustion SHALL produce unsure with its specific cause. Unsure SHALL never create a deduction automatically.

#### Scenario: Valid-looking evidence is fabricated
- **WHEN** the model cites a line number with text differing from the original source
- **THEN** the affected finding becomes unsure with an evidence-mismatch cause

#### Scenario: Model keeps returning malformed JSON
- **WHEN** the initial response and its one allowed schema retry fail validation
- **THEN** the checker records unsure and makes no additional schema retry

### Requirement: Enforced lab spending cap
The system SHALL enforce the configured per-lab budget, default USD 0.25, across concurrent requests, retries, and rechecks. It SHALL reserve a conservative maximum request cost before dispatch, reconcile usage afterward, and stop new dispatch when it cannot prove the cap will be respected. Remaining pairs SHALL become unsure with budget-exhausted causes.

#### Scenario: Parallel checks compete for remaining budget
- **WHEN** two requests cannot both fit within the remaining budget
- **THEN** atomic reservation admits only affordable work and routes the remaining work to unsure without exceeding the cap

#### Scenario: Request outcome or pricing is unknown
- **WHEN** a dispatched request times out without known usage, or configured pricing is unavailable
- **THEN** the reservation remains conservatively charged, or dispatch is refused, rather than assuming zero cost

### Requirement: Required rechecking after constraint changes
Every allowed pre-publication change to a constraint SHALL invalidate its previous active findings and decisions and trigger checking against the new revision for counted Accepted submissions. Deletion SHALL remove the constraint from active deductions and queue obligations while preserving history. Readiness SHALL remain false until current obligations are complete.

#### Scenario: Reviewed violation receives a lower deduction
- **WHEN** the TA reduces the constraint's deduction before publication
- **THEN** its prior findings and approvals are superseded, the revision is rechecked, and any newly required TA decisions must be completed before publication
