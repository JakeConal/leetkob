# lab-authoring Specification

## Purpose

Let an assigned teaching assistant create the real lab used for the PA#3 walkthrough, with its statement, tests, constraints and declared deductions.

## Requirements

### Requirement: Product-authorized lab creation

An assigned TA SHALL create a lab in their class through the product's screens or API, including statement, tests, constraints and TA-chosen deductions. Valid creation SHALL persist all required data atomically. The marked flow SHALL NOT require a lab seed script or manual database edit.

#### Scenario: PA3-LAB-01 Create the marking lab

- **WHEN** the assessor supplies the official sort-merge statement, tests and its two constraints through the authoring interface
- **THEN** a usable lab exists in that TA's class with exactly the supplied deductions and no seeded lab dependency

#### Scenario: PA3-LAB-02 Invalid or unauthorized creation

- **WHEN** creation targets another TA's class or contains invalid schedule, duplicate constraint IDs or unsupported limits
- **THEN** creation fails without a partial lab or tests becoming visible

### Requirement: Declared execution limits

The lab SHALL use C++17 with bounded CPU, container wall time, memory including swap, process count and output. The initial policy SHALL retain the repository's specified source, compile and per-test bounds. Users SHALL NOT supply compiler commands or filesystem paths. Submitted work SHALL be eligible only during the lab's declared window.

#### Scenario: PA3-LAB-03 Student reads the open lab

- **WHEN** an enrolled student reads the lab after opening
- **THEN** its statement, sample tests, constraints, deductions, latest revision date, limits and model-review notice are visible while hidden test contents remain private

### Requirement: Stable active requirements

Active tests and constraints SHALL have immutable input revisions. This checkpoint SHALL NOT expose an unreviewed editing path that can bypass revision invalidation. Model input notices SHALL explain sanitization and human approval before any deduction is released.

#### Scenario: PA3-LAB-04 Active requirements are probed for replacement

- **WHEN** a caller attempts an unsupported revision operation
- **THEN** it cannot replace the marked requirements or turn older findings into current evidence
