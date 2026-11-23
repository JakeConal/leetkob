# Quality gate workflow

`.github/workflows/quality.yml` runs the documented root commands and the
PA#3 tag-only sandbox qualification.

## Triggers

| Event               | Scope           |
| ------------------- | --------------- |
| `pull_request`      | targets `main`  |
| `push`              | `main`          |
| `push` tag          | `pa2` and `pa3` |
| Tag-only attack job | `pa3` tag only  |

The workflow keeps `permissions: contents: read`. It uses no repository
secret, deployment credential, model-provider key or write permission.

## Protected status contexts

The protected `main` ruleset requires these result names:

- `Lint`
- `Format`
- `Typecheck`
- `Test`

They match the job `name:` values in the workflow. Do not rename those jobs
or their required results. The tag-only `Sandbox attacks` job is additive and
is not a required pull-request context, so enabling it needs no branch
protection change.

## Ordinary pull-request checks

The required jobs and the build/security jobs install the pinned Node 22.13.0
and Go 1.27.4 toolchains and run `npm ci --ignore-scripts` first. The four
root commands stay identical to their documented local forms:

| Required result | Command                |
| --------------- | ---------------------- |
| `Lint`          | `npm run lint`         |
| `Format`        | `npm run format:check` |
| `Typecheck`     | `npm run typecheck`    |
| `Test`          | `npm test`             |

The `Test` job explicitly builds `leetkob-sandbox:pa3` and then runs the real
sandbox attack suite through `npm test`. The analyzer and checker container
tests are skipped there with:

```text
GOFLAGS="-p=1 -skip=TestDockerAnalyzerHardRuleTraps|TestCheckerImage"
```

Their pure-Go tests still run; packages run serially so Docker-backed suites
do not race for host resources. The skip keeps ordinary pull-request checks
inside the marker's 15-minute bound: a cold analyzer image build alone is
longer than the sane budget for a PR. The tag-only job below builds those
images and runs the complete suites, so containment coverage is never
removed from the submitted-revision run.

### Measured local reference

Measured on 2026-11-23 on Linux amd64, Docker 29.5.3, Go 1.27.4 and Node 22,
with dependency caches warm. The sandbox and analyzer image builds are
`--no-cache` cold builds; the checker build reuses the analyzer base image.

| Step                                             | Time    |
| ------------------------------------------------ | ------- |
| `npm run lint`                                   | 4.1 s   |
| `npm run format:check`                           | 4.1 s   |
| `npm run typecheck`                              | 6.8 s   |
| `npm test` (PR tag-gates, sandbox image built)   | 126.5 s |
| `npm test` (full container suites, images built) | 241.3 s |
| Sandbox attack suite (`go test -race`)           | 22.7 s  |
| Analyzer container trap suite                    | 14.3 s  |
| Checker image tests                              | 5.5 s   |
| Sandbox image cold build                         | 279 s   |
| Analyzer image cold build                        | 698 s   |
| Checker image build (analyzer base cached)       | 3.1 s   |

The sandbox build plus the test suite fits the 15-minute PR budget. The
analyzer cold build plus the same checks would not, which is why the analyzer
and checker container tests are tag-gated.

## Tag-only sandbox attack job

`Sandbox attacks` runs only for `refs/tags/pa3`. `actions/checkout` therefore
checks out the exact submitted tag revision, not `main` or a pull-request
merge commit. The job:

1. Fails closed if Docker or its daemon is unavailable.
2. Asserts that common provider-key environment variables are unset.
3. Builds the sandbox, analyzer and checker images from that revision.
4. Runs the complete `internal/sandbox` attack suite with `-race`.
5. Runs the analyzer container trap suite and the checker image tests, which
   use `--network none`, `MODEL_OFF=1`, no Docker socket and no provider key.
6. Fails if the sandbox or analyzer suites report a skip, and requires both
   checker image tests to pass.

Any containment, trap or image failure fails the job. Do not push `pa3` until
the submitted revision is final, and record only the actual tag run URL in
`PA3.md` after it exists.

## Local equivalent commands

Ordinary offline checks (the same tag-gated container tests as a pull request):

```bash
npm ci --ignore-scripts
npm run lint
npm run format:check
npm run typecheck
GOFLAGS="-p=1 -skip=TestDockerAnalyzerHardRuleTraps|TestCheckerImage" npm test
```

Full tag-equivalent qualification, once the images are available locally
(rebuild them offline only from already-present base images and packages):

```bash
docker build --tag leetkob-sandbox:pa3 apps/backend/sandbox
docker build --tag leetkob-analyzer:pa3 apps/backend/analyzer
docker build -f apps/backend/checker/Dockerfile --tag leetkob-checker:pa3 apps/backend

(cd apps/backend && go test -race -count=1 -v ./internal/sandbox/)
(cd apps/backend && go test -race -count=1 -v ./internal/analyzer/)
(cd apps/backend && go test -race -count=1 -v ./internal/checker/)
```

Do not set `MODEL_API_KEY` or any other provider credential for those
commands. The checker tests already run the image with `MODEL_OFF=1` and
`--network none`.
