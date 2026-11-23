# Spec Delta

## Purpose

Produce constraint-level evidence for Accepted submissions using deterministic rules first and bounded semantic model calls, without allowing a model to grade students.

## ADDED Requirements

### Requirement: Generic hard rules before model dispatch

Ban lists starting with Do not use or Do not call SHALL resolve every backticked banned name through syntax-aware rules. A decided hard-rule pair SHALL NOT call the model. Rules and prompts SHALL NOT contain dev-problem-specific answers. The model-off dev run SHALL decide every ban-list pair with zero wrongful deductions.

#### Scenario: PA3-CHECK-01 Compliant traps

- **WHEN** a banned name appears only in a comment/string, a student's namespace defines sort, or algorithm is included only for std::swap
- **THEN** the hard rule does not report a false violation and makes no semantic-model call for that decided pair

#### Scenario: PA3-CHECK-02 Indirect banned call

- **WHEN** a forbidden library call is resolved through a supported namespace alias, using declaration, macro or wrapper
- **THEN** it yields violation with original-line evidence rather than a raw text match or a hard-coded dev answer

### Requirement: Sanitized real semantic calls

Every eligible semantic constraint SHALL use the real configured model within the enforced budget. Input SHALL contain only statement, constraints and code stripped of comments and student identity. Unsafe sanitization SHALL become unsure without transmission. Each result SHALL be pass, violation or unsure with evidence; the model SHALL NOT set deductions or grades.

#### Scenario: PA3-CHECK-03 Injection and identity input

- **WHEN** source includes an injection comment and a known student name or ID
- **THEN** the sanitized prompt excludes comments/identity, treats remaining code as data and retains original-line mapping

### Requirement: Schema evidence and timeout limits

Model output SHALL be checked against a strict schema and original-source evidence. Invalid schema, unknown fields, fabricated evidence or timeouts SHALL produce unsure rather than an accepted deduction. Invalid-output retry SHALL be bounded to one. Unit and contract tests SHALL use fake providers with no network.

#### Scenario: PA3-CHECK-04 Fake model emits forbidden data

- **WHEN** a fake model emits a grade, unknown constraint, duplicate/missing result or invalid evidence
- **THEN** validation rejects it, records the applicable unsure cause and makes no unbounded retry

#### Scenario: PA3-CHECK-05 Fake model times out

- **WHEN** a fake provider exceeds the configured call deadline
- **THEN** the check finishes as unsure and the test requires neither a provider key nor network

### Requirement: Enforced lab spending cap

The checker SHALL reserve conservative priced cost atomically before dispatch and include retries/rechecks within the configured per-lab cap, default USD 0.25. Unknown usage SHALL remain charged and unknown pricing SHALL refuse dispatch. Exhausted budget SHALL produce unsure without another provider request.

#### Scenario: PA3-CHECK-06 Concurrent calls compete

- **WHEN** remaining budget cannot cover two concurrent requests
- **THEN** only affordable work is dispatched and total settled plus reserved spend cannot exceed the cap

### Requirement: Auditable provider call provenance

The committed real-model dev run SHALL include sanitized prompts, provider request IDs, model, tokens, elapsed time and cost for each call. The call evidence SHALL show no dispatch for hard-rule-decided pairs. Operational logs and student APIs SHALL NOT expose prompts, raw responses, identity or credentials.

#### Scenario: PA3-CHECK-07 Real run is audited

- **WHEN** a reviewer follows the committed call-log references
- **THEN** actual provider request IDs and usage/cost records support semantic dispatch and no mock run is represented as real
