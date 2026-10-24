# Spec Delta

## Purpose

Defines the reproducible repository checks, secret-safety controls, protected-branch behavior, and auditable evidence that keep LeeTKOB changes trustworthy.

## ADDED Requirements

### Requirement: Reproducible local checks
The repository SHALL document one command each for lint, format verification, static type checking, and automated tests, and each command SHALL succeed in a fresh supported environment after documented setup.

#### Scenario: Contributor runs the documented harness
- **WHEN** a contributor follows the README setup and runs each documented check command
- **THEN** lint, format verification, type checking, and tests complete successfully without undocumented preparation

### Requirement: CI command parity
Pull-request CI SHALL install from the lockfile and execute the same four commands documented for local use.

#### Scenario: Pull request runs quality checks
- **WHEN** a pull request targets `main`
- **THEN** CI runs separate required results for lint, format verification, type checking, and tests using the documented commands

#### Scenario: Contract test fails
- **WHEN** a change violates a documented endpoint behavior
- **THEN** the test result is red and the pull request cannot merge

### Requirement: Protected main branch
The public repository SHALL protect `main`, require the four current quality results, require pull-request review flow, and prevent direct contributor pushes after protection is enabled.

#### Scenario: Required check is red
- **WHEN** any required quality result is failing or missing
- **THEN** GitHub reports the pull request as blocked from merging

#### Scenario: All required checks are green
- **WHEN** every required quality result succeeds and all configured review requirements are met
- **THEN** GitHub permits the pull request to merge through the configured strategy

### Requirement: Repository-specific agent rules
The repository SHALL provide a concise agent-readable rules file with `Stack`, `Commands`, `Conventions`, and `Never` sections plus `Allowed`, `Ask first`, and `Never` permission tiers.

#### Scenario: Agent begins repository work
- **WHEN** an assistant reads repository guidance
- **THEN** it can identify the supported stack, exact checks, coding conventions, prohibited product actions, and operations requiring user approval

### Requirement: Secret-safe configuration
The repository SHALL ignore local environment files, provide an example containing every supported variable with no secret values, and contain no credentials in the submitted tree or reachable Git history.

#### Scenario: Fresh configuration
- **WHEN** a contributor prepares local configuration
- **THEN** every required variable is discoverable from `.env.example` without revealing a credential

#### Scenario: Secret audit
- **WHEN** the repository tree and reachable history are scanned for known credential patterns
- **THEN** no live or historical credential is found

### Requirement: Capped model budget policy
The README and AI log SHALL name the intended model integration policy, state a numeric per-lab spending cap, and state that unit and contract tests never perform real model calls.

#### Scenario: Reviewer checks model-cost controls
- **WHEN** a reviewer reads the repository documentation
- **THEN** the spending cap and no-real-model-test rule are explicit even though PA#2 does not yet call a model

### Requirement: Quality-gate evidence trail
The root evidence file SHALL link the public repository, protected-branch settings evidence, a pull request blocked by a red contract test, the repair commit that made it green, the tag CI run, and the deliberate contract-breaking run.

#### Scenario: Lecturer audits the gate
- **WHEN** the lecturer opens `PA2.md`
- **THEN** every required item resolves to specific public evidence rather than an unsupported claim

### Requirement: Submission state
The submitted commit SHALL be tagged `pa2`, the tag SHALL exist on the public remote, and a repository archive including `.git` SHALL use the required student-ID and self-assessed-total filename.

#### Scenario: Submitted revision is reproduced
- **WHEN** the lecturer checks out tag `pa2` and follows the README
- **THEN** the four required checks run against the exact self-assessed revision

#### Scenario: Submission archive is inspected
- **WHEN** the archive is extracted
- **THEN** it contains the repository, `.git`, `PA2.md`, `AI-LOG.md`, and `SELF_ASSESSMENT_REPORT.md`
