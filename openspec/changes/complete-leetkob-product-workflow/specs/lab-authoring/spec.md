# Spec Delta

## Purpose

Defines how assigned teaching assistants prepare labs, publish their submission requirements, and revise those requirements fairly throughout a lab's lifecycle.

## ADDED Requirements

### Requirement: Lab schedule and ownership
An assigned TA SHALL create a lab with a Markdown statement, opening time, submission deadline, and owning class. The opening time SHALL precede the deadline. Students SHALL see the lab from its opening time; the submission deadline SHALL be immutable after opening under this baseline policy.

#### Scenario: Lab opens for its class
- **WHEN** server time reaches the configured opening time
- **THEN** enrolled students can read the statement, sample tests, constraints, deductions, and run limits

#### Scenario: Invalid or unauthorized authoring
- **WHEN** a TA creates a lab outside their class or supplies a deadline at or before opening
- **THEN** the system rejects the operation without creating a lab

### Requirement: Test bundle validation
The system SHALL accept versioned ZIP test bundles containing matched input and expected-output files, with explicit sample and hidden test classification. It SHALL reject unsafe paths, links, excessive expansion, and invalid or incomplete pairs before activating a bundle.

#### Scenario: Bundle contains path traversal
- **WHEN** a test ZIP contains an absolute path, parent traversal, or a symbolic link
- **THEN** ingestion fails without writing outside isolated staging storage

#### Scenario: Hidden test remains private
- **WHEN** a student reads a lab or its judging results
- **THEN** sample tests are available in the statement but hidden inputs, expected outputs, and actual test outputs are absent

### Requirement: Visible constraints and submission notice
Each constraint SHALL have a stable identifier, text, and a deduction from 0 through 10. From opening, students SHALL see every constraint and deduction, the latest revision date, and a notice describing deterministic/model checking, sanitized model input, and TA approval before grades are released.

#### Scenario: Student prepares a submission
- **WHEN** the student opens the submission form
- **THEN** it explains that only statement, constraints, and code stripped of comments, names, and student IDs are sent to the model, and that the model does not decide grades

### Requirement: Constraint revision restrictions
Before opening, the TA SHALL freely add, remove, and edit constraints. From opening until publication, the system SHALL prohibit additions and deduction increases, allow clarification without meaning change, reductions, and deletion, and retain revision history. After publication, all constraint edits SHALL be rejected.

#### Scenario: TA relaxes an open lab constraint
- **WHEN** an assigned TA reduces a deduction, deletes a constraint, or confirms a wording clarification preserves meaning
- **THEN** the system records the old and new revision, actor, time, and reason, exposes the revision date, and invalidates the affected old findings and decisions

#### Scenario: TA attempts a stricter rule
- **WHEN** a lab is open and a TA adds a constraint or increases its deduction
- **THEN** the system rejects the edit; the editor also states that meaning-changing wording is prohibited even though semantic equivalence requires TA judgment

#### Scenario: Constraint edit races publication
- **WHEN** publication and an allowed constraint revision are requested concurrently
- **THEN** one commits first and the other observes the resulting state, so a published lab cannot contain an unreviewed revision

### Requirement: Configurable execution limits
Labs SHALL use C++17 with `g++ -std=c++17 -O2`, a 64 KiB source limit, 1 second CPU per test by default adjustable to 1-5 seconds, wall time three times CPU, and 256 MiB memory adjustable to 64-512 MiB. Compile time SHALL be limited to 10 seconds, processes including threads to 16, and output per test to 1 MiB.

#### Scenario: TA configures valid limits
- **WHEN** a TA chooses 2 seconds CPU and 512 MiB memory
- **THEN** the lab uses 6 seconds wall time per test and retains the fixed compiler, compile timeout, process, output, and source limits

#### Scenario: Unsupported limit is requested
- **WHEN** a TA requests values outside the allowed range or changes the compiler command
- **THEN** the operation fails without activating an unsupported execution policy

### Requirement: Reference validation and reusable authoring
The enhancement phase SHALL offer constraint templates, cloning an authorized lab into a new draft, and a pre-opening reference-solution check. Cloning SHALL exclude student data, decisions, grades, and publication state. Reference validation SHALL report whether the solution is Accepted and every constraint is pass.

#### Scenario: Reference solution fails validation
- **WHEN** the sample solution fails a test or receives violation or unsure on a constraint
- **THEN** the TA sees a warning and the failing results before opening the lab

#### Scenario: TA reuses a lab
- **WHEN** a TA clones a lab into another class they own
- **THEN** an independent draft copies authoring content with new identifiers and requires a new schedule
