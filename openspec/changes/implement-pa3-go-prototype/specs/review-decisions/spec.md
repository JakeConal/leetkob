# Spec Delta

## Purpose

Keep checker evidence available to assigned teaching assistants for deliberate review while preventing unconfirmed findings from reaching student grades.

## ADDED Requirements

### Requirement: Evidence-centered private queue

The assigned TA's queue SHALL show each current eligible submission/constraint verdict, source kind, original evidence, unsure cause and TA-configured deduction. Students and other classes' TAs SHALL NOT receive these findings. Every violation/unsure and a persisted random ceil(10%) of pass pairs SHALL require review.

#### Scenario: PA3-REVIEW-01 Three marking submissions

- **WHEN** compliant, violating and injection submissions finish the real pipeline
- **THEN** the assigned TA can inspect per-constraint results, source evidence and configured deductions while another TA receives concealed 404

#### Scenario: PA3-REVIEW-02 Queue refresh

- **WHEN** an unchanged queue is refreshed
- **THEN** required pass-sample membership stays stable and all unresolved violations/unsure remain obligations

### Requirement: Deliberate revision-bound TA decisions

The TA SHALL explicitly choose pass or violation with a reason and current finding revision. Violation SHALL require matching original-line evidence. Unsure SHALL have no default decision. Store decisions and audit actor/time separately from immutable checker provenance; reject stale decisions atomically.

#### Scenario: PA3-REVIEW-03 Resolve timeout unsure

- **WHEN** the TA opens an unsure finding caused by timeout
- **THEN** original source/constraint remain available and no deduction becomes confirmed without an explicit valid decision

#### Scenario: PA3-REVIEW-04 Concurrent decisions

- **WHEN** two TAs submit decisions based on the same previous finding revision
- **THEN** only one commits and the stale request receives a conflict instead of overwriting it

### Requirement: No automatic grade publication

Model findings SHALL NOT change a student's mark or reach the student as a deduction without TA confirmation. This PA#3 implementation SHALL leave production mark publication unavailable. The existing compatibility Publish handler SHALL use fixture-only storage and SHALL NOT access production labs.

#### Scenario: PA3-REVIEW-05 Student reads before publication

- **WHEN** a student queries their result or guesses the review route after model checking
- **THEN** only authorized judge status is available, with no provisional deduction, model verdict or review evidence, following FAQ-08

#### Scenario: PA3-REVIEW-06 Publication bypass is attempted

- **WHEN** a caller attempts legacy or new production publication while findings are unreviewed
- **THEN** no production marks are published; any future implemented Publish route must refuse pending reviews before it can be enabled
