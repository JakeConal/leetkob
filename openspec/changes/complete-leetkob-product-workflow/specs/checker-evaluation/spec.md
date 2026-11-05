# Spec Delta

## Purpose

Defines a reproducible, dataset-independent checker evaluation that measures false deductions and evidence quality before checker changes reach production.

## ADDED Requirements

### Requirement: Dataset-independent checker command
The checker SHALL provide one documented command accepting a dataset directory and prediction output path. It SHALL consume statements, constraints, and solution source without per-problem configuration and SHALL write one JSONL record per solution-constraint pair, using the official solution identifiers, verdict enum, and evidence schema.

#### Scenario: Checker receives unseen problems
- **WHEN** a dataset with new problem IDs follows the official directory layout
- **THEN** the same command emits predictions without code, prompt, or configuration changes tied to those IDs

#### Scenario: Labels exist beside the dataset
- **WHEN** the checker runs on the dev or team set
- **THEN** prediction generation does not read labels or scoring tags; only the separate evaluation step consumes ground truth

### Requirement: Official scoring thresholds
Evaluation SHALL use the lecturer's unmodified scorer on the pinned dev set and a separate team set. Passing SHALL require zero pass-labelled pairs predicted violation, recall at least 90% with valid labelled-line evidence, and unsure rate at most 20%. Any nonzero scorer exit SHALL fail the evaluation.

#### Scenario: Correct verdict has wrong evidence
- **WHEN** a violation prediction has no labelled evidence line or has incorrect original text
- **THEN** the scorer does not count the violation as caught

#### Scenario: Prediction file is incomplete or malformed
- **WHEN** pairs are missing
- **THEN** they count as unsure; malformed JSON, unknown IDs, duplicate pairs, or invalid evidence instead cause the scorer's invalid-input failure

### Requirement: Revision-bound reproducibility
Evaluation evidence SHALL identify source revision, dataset/scorer hashes, checker and prompt versions, provider/model configuration, predictions, metrics, execution time, and cost. Evidence from a different relevant revision or configuration SHALL NOT satisfy the current merge gate.

#### Scenario: Prompt changes after a passing evaluation
- **WHEN** a new commit changes the prompt after evidence was recorded
- **THEN** the prior evidence no longer satisfies that revision's required evaluation result

### Requirement: Trustworthy team cases
The team dataset SHALL remain separate from lecturer data. Each labelled solution SHALL compile with C++17 and pass every problem test using token comparison before inclusion. Ground truth SHALL be reviewed independently of checker predictions and SHALL NOT be altered to make the gate pass.

#### Scenario: Rejected finding suggests a new case
- **WHEN** the enhancement flow nominates a TA-rejected finding or successful appeal
- **THEN** it becomes an anonymized candidate requiring permission to reuse, Accepted validation, and human label review before inclusion

### Requirement: Isolated paid evaluation execution
Unit and contract tests SHALL use deterministic substitutes and make no real model requests. Live evaluation SHALL run only in an explicitly authorized, budgeted environment that keeps credentials outside untrusted candidate execution. Pull requests SHALL be unable to read model credentials, and lack of valid live evidence SHALL fail closed for affected changes.

#### Scenario: Untrusted pull request changes checker code
- **WHEN** the changed checker is evaluated
- **THEN** its execution receives no live credential, and model access is mediated by the trusted budgeted evaluation boundary

#### Scenario: Authorized evaluation budget is exhausted
- **WHEN** a live evaluation cannot finish within its authorized cap
- **THEN** evidence reports the incomplete or failed result and the gate does not substitute an old passing report
