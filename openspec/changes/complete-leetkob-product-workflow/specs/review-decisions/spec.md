# Spec Delta

## Purpose

Defines the human review obligations that turn checker findings into defensible grading decisions for the current counted submissions of a lab.

## ADDED Requirements

### Requirement: Complete current review queue
The queue SHALL include every current violation, every current unsure, and a random sample of `ceil(0.10 * P)` current pass findings, where P counts submission-constraint pairs in the lab. Sampling SHALL select without replacement and persist its selection for the active finding generation.

#### Scenario: Small pass population
- **WHEN** a lab generation contains one through ten pass findings
- **THEN** exactly one is selected for mandatory pass review

#### Scenario: Pass population changes
- **WHEN** a counted submission or active revision changes the population
- **THEN** the system creates a new persisted sample for that generation, preserves decisions for unchanged findings where still applicable, and requires review of newly sampled undecided findings

#### Scenario: Queue is refreshed repeatedly
- **WHEN** a TA reloads an unchanged lab queue
- **THEN** its sample membership does not change and no required review can be evaded by refreshing

### Requirement: Evidence-centered TA decisions
Assigned TAs SHALL see a finding's constraint, source, reason, original code, and evidence. Unsure SHALL expose its specific cause and require an explicit pass or violation decision with no default. Every decision of violation SHALL include at least one valid original evidence line.

#### Scenario: TA resolves a timeout
- **WHEN** a TA opens an unsure finding caused by a provider timeout
- **THEN** the code and constraint remain available even without model evidence, and the TA must explicitly choose a verdict

#### Scenario: TA attempts unsupported deduction
- **WHEN** a TA selects violation without valid original line evidence
- **THEN** the system rejects the decision and leaves the required item unresolved

### Requirement: Deliberate grouped review
The system SHALL group findings by source and constraint. Deterministic violations SHALL support explicit group approval with access to individual evidence. Model violations SHALL require individual decisions with visible evidence; unsure SHALL require opening the individual submission. There SHALL be no approve-all action for model violations or unsure.

#### Scenario: TA approves deterministic findings as a group
- **WHEN** a TA confirms a group of current deterministic violations
- **THEN** each finding receives its own recorded decision and the operation rejects stale group membership rather than approving hidden replacements

#### Scenario: TA reviews model findings
- **WHEN** a TA selects a semantic-violation group
- **THEN** the interface shows each finding's reason and evidence and requires a decision per finding

### Requirement: Overrides and decision history
Before publication, assigned TAs SHALL inspect every counted submission and override any finding, including unsampled pass findings. Every decision SHALL append actor, timestamp, old and new verdict, reason, evidence, and finding revision to audit history; checker provenance SHALL remain available.

#### Scenario: TA catches an unsampled violation
- **WHEN** a TA changes an unsampled pass to violation with valid evidence and reason
- **THEN** the deduction becomes eligible for grade calculation and the original checker verdict remains in history

### Requirement: Concurrent review consistency
Decision requests SHALL identify the finding revision being reviewed. Stale, superseded, or conflicting decisions SHALL be rejected without overwriting another TA's work. After publication, review decisions SHALL be immutable and changes SHALL use the individual correction or appeal flow.

#### Scenario: Two TAs decide the same finding
- **WHEN** both submit decisions based on the same earlier revision
- **THEN** one commits and the other receives a conflict with an invitation to refresh

#### Scenario: Review and publication race
- **WHEN** the last required review and publication execute concurrently
- **THEN** publication either observes the committed decision or fails readiness; it cannot publish an intermediate decision state
