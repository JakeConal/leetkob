# Spec Delta

## Purpose

Defines transparent grades, controlled student feedback, and accountable corrections and appeals after an entire lab has been published.

## ADDED Requirements

### Requirement: Deterministic grade calculation
For counted Accepted submissions, the system SHALL calculate `max(0, 10 - sum(confirmed deductions))`, taking each active violated constraint's deduction once regardless of evidence count. Counted non-Accepted submissions and students with no submission SHALL default to zero. Models SHALL never set deduction amounts or grades.

#### Scenario: Repeated violation of one constraint
- **WHEN** C1 deducts 3 and is violated twice, and C2 deducts 4 and is violated once
- **THEN** the unadjusted grade is 3 with two deductions and all relevant evidence retained

#### Scenario: Deductions exceed ten
- **WHEN** confirmed deduction amounts total more than 10
- **THEN** the grade is zero rather than negative

### Requirement: Student grade visibility
Before publication, students SHALL see judge results only. After publication, they SHALL see their own current grade and confirmed deductions, each with constraint, amount, original evidence lines, and reason. Rejected proposals, unsure verdicts, and sample membership SHALL never be exposed to students.

#### Scenario: Student probes a grade API before publication
- **WHEN** a student requests grade or review details for an unpublished lab
- **THEN** no provisional grade, deduction, checker verdict, or review evidence is disclosed

#### Scenario: Published student feedback
- **WHEN** a student opens their published result
- **THEN** they see the counted submission and current confirmed deductions without private review history or another student's data

### Requirement: Audited individual grade adjustments
Assigned TAs SHALL adjust an individual published grade to a value from 0 through 10 with a nonempty reason. The system SHALL retain previous grades, actor, time, and reason and make the new grade and reason visible immediately. The enhancement phase SHALL also allow reasoned manual scores for non-Accepted work before publication.

#### Scenario: TA corrects a published grade
- **WHEN** a TA submits a valid reasoned correction for a student in their class
- **THEN** the current grade changes atomically with audit history while publication time and other students' grades remain unchanged

#### Scenario: Invalid manual score
- **WHEN** a score falls outside 0-10 or its reason is empty
- **THEN** no grade or audit mutation commits

### Requirement: Single deduction appeal within seven days
A student SHALL appeal each confirmed deduction on their own published submission at most once, with a nonempty explanatory sentence, while `publishedAt <= serverTime < publishedAt + 7 days`. Judge verdicts and manually entered grades SHALL NOT be appealable through this flow. Individual corrections SHALL NOT restart the appeal window.

#### Scenario: Student appeals two deductions
- **WHEN** a student challenges two eligible deductions during the window
- **THEN** two independent pending appeals are created, each tied to its own deduction

#### Scenario: Duplicate or expired appeal
- **WHEN** a student repeats an appeal for a previously appealed deduction or submits at the seven-day boundary
- **THEN** the request is rejected without reopening or duplicating an appeal

### Requirement: Atomic reasoned appeal resolution
Assigned TAs SHALL accept or reject pending appeals with a reason. Acceptance SHALL remove the disputed deduction and update the grade atomically; rejection SHALL preserve it. Neither outcome SHALL permit student reopening. Concurrent resolution and manual correction SHALL be serialized with complete history.

#### Scenario: Appeal is accepted
- **WHEN** the TA confirms the student did not violate the appealed constraint
- **THEN** the deduction is removed, the grade is recalculated with any explicit manual adjustment, and the student sees the outcome and reason together

#### Scenario: Appeal was rejected but TA later finds an error
- **WHEN** the student tries to reopen the appeal
- **THEN** reopening is denied, while a TA can still issue an independently audited correction

### Requirement: Gradebook and review reports
The enhancement phase SHALL provide assigned TAs a class gradebook, CSV export, and lab reports covering score distribution, findings by type/source, unsure rate, and TA rejection rate. Reports SHALL define their denominators and version scope; exports SHALL neutralize spreadsheet formula injection.

#### Scenario: TA exports a class gradebook
- **WHEN** an assigned TA requests the export
- **THEN** it contains only that class's authorized data, identifies provisional versus published grades, and encodes untrusted spreadsheet cells safely

### Requirement: Durable publication and appeal notifications
The enhancement phase SHALL notify students in-app and by email when grades are published or their appeal is resolved. Notifications SHALL be generated only after a successful state commit, tolerate retries without duplicate in-app events, and link to authenticated details without putting code or evidence in email.

#### Scenario: Notification provider is unavailable
- **WHEN** a grade or appeal update commits but email delivery fails
- **THEN** the result remains visible in-app and email retries without reversing the business operation
