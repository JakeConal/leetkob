# PA#2 Evidence Index

This file is the evidence map for the `pa2` submission of LeeTKOB. The submitted tag is intended to be checked out and verified with the commands in `README.md`.

## Repository and protected branch

- Repository: <https://github.com/JakeConal/leetkob>
- Protected branch: `main`
- Protection API: <https://api.github.com/repos/JakeConal/leetkob/branches/main/protection>
- Captured settings: [`docs/evidence/main-branch-protection.json`](docs/evidence/main-branch-protection.json)
- Required checks: `Lint`, `Format`, `Typecheck`, and `Test`, with strict up-to-date enforcement
- Additional protections: pull-request flow, administrator enforcement, stale-review dismissal, conversation resolution, and disabled force pushes/deletions

## Gate-bites demonstration

The same protected pull request contains the deliberate regression and its repair. The contract and tests were unchanged throughout the demonstration.

| Evidence                | Link                                                                                                                               | What it proves                                                                                                               |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Protected pull request  | [PR #2](https://github.com/JakeConal/leetkob/pull/2)                                                                               | GitHub reported the pull request as blocked while required `Test` was red; it was merged only after repair and green checks. |
| Deliberate break commit | [`4e808f7a96d46c0c67d794b5f452866162ace116`](https://github.com/JakeConal/leetkob/commit/4e808f7a96d46c0c67d794b5f452866162ace116) | Implementation-only change allowed publication with pending reviews.                                                         |
| Failing workflow run    | [Quality Gate run 40078757471](https://github.com/JakeConal/leetkob/actions/runs/40078757471)                                      | The deliberate regression made the required Test job fail.                                                                   |
| Failing required job    | [Test job 120827694233](https://github.com/JakeConal/leetkob/actions/runs/40078757471/job/120827694233)                            | Contract tests detected the pending-review violation.                                                                        |
| Repair commit           | [`bf6cb02cc6d0b5e336de97fb121b9060000382cc`](https://github.com/JakeConal/leetkob/commit/bf6cb02cc6d0b5e336de97fb121b9060000382cc) | Restored the implementation without changing the contract or tests.                                                          |
| Green workflow run      | [Quality Gate run 40078849258](https://github.com/JakeConal/leetkob/actions/runs/40078849258)                                      | All six repository jobs and SonarCloud passed on the repaired pull request.                                                  |
| Protected merge         | [`fd74035535e83049665692953cf16a6aaf47cbaa`](https://github.com/JakeConal/leetkob/commit/fd74035535e83049665692953cf16a6aaf47cbaa) | The repaired pull request entered `main` through a merge commit.                                                             |

## Contract and implementation

- Client contract: [`docs/contract/publish-lab.md`](docs/contract/publish-lab.md)
- Contract tests: [`apps/api/test/contract/publish-lab.contract.test.ts`](apps/api/test/contract/publish-lab.contract.test.ts)
- Framework-independent publication rule: [`apps/api/src/domain/publication-service.ts`](apps/api/src/domain/publication-service.ts)
- NestJS/Fastify HTTP boundary: [`apps/api/src/http`](apps/api/src/http)
- Rules and permission tiers: [`AGENTS.md`](AGENTS.md)
- Assistant decisions: [`AI-LOG.md`](AI-LOG.md)
- Rubric scoring: [`SELF_ASSESSMENT_REPORT.md`](SELF_ASSESSMENT_REPORT.md)

## Submitted tag and CI

- Tag: [`pa2`](https://github.com/JakeConal/leetkob/tree/pa2)
- Verified tag run: [Quality Gate run 40152904321](https://github.com/JakeConal/leetkob/actions/runs/40152904321)
- Tag-triggered workflow: [Quality Gate runs filtered to `pa2`](https://github.com/JakeConal/leetkob/actions/workflows/quality.yml?query=branch%3Apa2)

The workflow listens to pushes of the exact `pa2` tag. The immutable run above records the first verified tag push; the filtered link remains stable and exposes the current tag run and its individual jobs after final evidence updates.
