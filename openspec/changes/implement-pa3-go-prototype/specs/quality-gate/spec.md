# Spec Delta

## MODIFIED Requirements

### Requirement: Reproducible local checks

The repository SHALL document one root command each for lint, format verification, static type checking and automated tests. Each SHALL cover the existing API/web workspaces and applicable Go API/worker/checker packages and succeed after documented clean setup. Go formatting, vet, builds, tests, supported race checks and pinned vulnerability scanning SHALL have explicit coverage without weakening existing checks.

#### Scenario: Contributor runs the documented harness

- **WHEN** a contributor follows the README setup and runs each documented check command
- **THEN** lint, format verification, type checking, and tests cover both applications and complete successfully without undocumented preparation

#### Scenario: PA3-GATE-01 Go package fails

- **WHEN** an API, worker or checker package has a compile/test/static-check failure
- **THEN** the applicable root command fails rather than silently checking only the Node workspaces

### Requirement: CI command parity

Pull-request CI SHALL install pinned dependencies and execute the same four root commands documented for local use, preserving their required-result names and existing PA#2 suites. CI SHALL additionally build all Go commands, run supported race checks and exercise the real sandbox attacks on the pa3 tag without model-provider access.

#### Scenario: Pull request runs quality checks

- **WHEN** a pull request targets main
- **THEN** CI runs separate required results for lint, format verification, type checking and tests using the documented commands

#### Scenario: Contract test fails

- **WHEN** a change violates a documented endpoint behavior
- **THEN** the test result is red and the pull request cannot merge

#### Scenario: PA3-GATE-02 Submitted tag runs attacks

- **WHEN** pa3 is pushed
- **THEN** the recorded tag run exercises all nine sandbox controls on the actual submitted revision

## ADDED Requirements

### Requirement: Focused test-first implementation history

Hard-rule tests including traps and schema/timeout tests SHALL be committed before or with the code they test. Each implementation commit SHALL cover one coherent part. Tests, eval cases and labels SHALL NOT be weakened to obtain passing checks, and history SHALL NOT be reconstructed afterward to claim tests-first work.

#### Scenario: PA3-GATE-03 Lecturer inspects history

- **WHEN** the submitted history and AI-LOG are inspected
- **THEN** tests and focused sandbox/queue/rules/model/limits commits reflect their actual creation order and recorded changed/rejected decisions

### Requirement: Reachable PA3 deployment and evidence index

The release SHALL be publicly reachable from the PA#3 deadline until PA#4 marks are released. PA3.md SHALL link the URL, five synthetic test accounts, pa3 sandbox CI run, checker/Docker/model-off commands, committed real-model run and at-most-five-minute fallback recording. Paid evaluation and external deployment SHALL retain explicit authorization boundaries.

#### Scenario: PA3-GATE-04 Assessor follows PA3.md

- **WHEN** the assessor follows each evidence reference on the submitted tag
- **THEN** they can sign in, create the marked lab and repeat the flow or use the fallback recording, with predictions/report/call provenance tied to that revision

### Requirement: Honest PA3 submission evidence

The public submitted revision SHALL have a pushed pa3 tag and a repository archive including .git, PA3.md, AI-LOG.md and SELF_ASSESSMENT_REPORT.md. The archive name SHALL use ascending team student IDs and the evidenced self-assessed total. Assessment SHALL have one evidence-backed row per rubric criterion and disclose unfinished work. Coursework upload SHALL require explicit user instruction.

#### Scenario: PA3-GATE-05 Archive is reproduced

- **WHEN** the final archive is extracted
- **THEN** its tag, Git history, required files, self-assessment total and filename agree without fabricated CI, deployment or model evidence
