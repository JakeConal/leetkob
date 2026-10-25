# LeeTKOB

LeeTKOB is a teaching-assistant workflow for checking whether accepted C++ submissions follow lab constraints before marks are published. PA#2 establishes the repository harness, implements the contract of the irreversible publication step, and provides a reviewer-facing status dashboard.

## PA#2 scope

This checkpoint includes:

- the contract and executable behavior of `POST /labs/{labId}/publish`;
- signed PA#2 bearer claims, TA role and class ownership checks;
- submission-deadline and pending-review protection plus exact-response idempotency;
- RFC 9457 problem details and contract tests;
- a NestJS API using Fastify and a responsive Next.js reviewer dashboard using TanStack Query;
- identical local and pull-request quality commands.

It deliberately does not yet include durable storage, OAuth/OIDC, the C++ sandbox, queues, model calls, live review data, the evaluation dataset, or deployment. Those belong to PA#3 and later milestones. The dashboard review rows are explicitly demonstration data and contain no student identity or source code.

## Architecture

The repository is an npm-workspace modular monolith:

- `apps/api` contains the NestJS 12 application, Fastify transport, framework-independent publication domain, signed-claim boundary, in-memory adapters, and contract tests.
- `apps/web` contains the Next.js 16 App Router application. Most of the page is server-rendered; a small TanStack Query client boundary owns API-health loading, retry, and error states.
- `docs/design/theme.md` is the authoritative Swiss Minimalist UI and layout system.

## Requirements

- Node.js 22.13 or newer
- npm 10 or newer
- Git

## Fresh-clone setup

```sh
git clone https://github.com/JakeConal/leetkob.git
cd leetkob
npm ci --ignore-scripts
cp .env.example .env
```

Set `AUTH_TOKEN_SECRET` to a locally generated high-entropy value before starting the server. Never commit `.env`.

Start the development process:

```sh
npm run dev
```

The dashboard starts at `http://127.0.0.1:3000` and the API starts at `http://127.0.0.1:3001`. Set `API_PORT`, `WEB_ORIGIN`, and `NEXT_PUBLIC_API_BASE_URL` when those defaults need to change. You can also run one side at a time with `npm run dev:web` or `npm run dev:api`.

## Quality commands

These exact commands run locally and as separate GitHub required checks:

| Check                   | Command                |
| ----------------------- | ---------------------- |
| Lint                    | `npm run lint`         |
| Format verification     | `npm run format:check` |
| Static type check       | `npm run typecheck`    |
| Unit and contract tests | `npm test`             |

Each root command covers both workspaces where applicable. `npm run build` verifies both production builds. The additional security job runs `npm run security`, which scans the current tree and reachable Git history for high-confidence credential patterns and runs `npm audit --audit-level=high`.

## Publication contract

The standalone client contract is in [`docs/contract/publish-lab.md`](docs/contract/publish-lab.md). The route requires:

- `Authorization: Bearer <signed-claim>`;
- `Idempotency-Key: <8-128 printable ASCII characters>`;
- an empty request body.

Publication is allowed only when server time is equal to or later than the lab's submission deadline and every review-queue item has a TA decision. This follows [product-owner FAQ-12](https://github.com/tkob-team/LeeTKOB-FAQ/blob/2b82b90ca3696fa81c68464f8a51ceb86406fef6/FAQ.md#faq-12). A pre-deadline attempt returns the documented `lab-still-open` RFC 9457 problem, and a same-key retry replays that exact response.

The PA#2 HMAC claim verifier is an isolated boundary for deterministic testing. Replace it with the deployment identity provider before production; callers never provide trusted role or class headers.

`GET /health` returns the safe API status used by the dashboard. It does not expose configuration, credentials, student data, or storage details.

## Model cost and secret policy

PA#3 is intended to use the OpenAI provider with a small configurable model. The enforced policy is a maximum **USD 0.25 per lab**, configured by `MODEL_SPENDING_CAP_USD`; processing must stop or return `unsure` before exceeding it. The exact PA#3 model remains configurable through `MODEL_NAME`.

PA#2 performs **no model calls**. Unit tests and contract tests must never make a real model request. `MODEL_API_KEY` stays only in an ignored local or deployment environment and must never enter Git, logs, tests, the code-execution sandbox, or API responses.

## Repository evidence

- PA#2 evidence index: [`PA2.md`](PA2.md)
- Assistant decisions: [`AI-LOG.md`](AI-LOG.md)
- Rubric self-assessment: [`SELF_ASSESSMENT_REPORT.md`](SELF_ASSESSMENT_REPORT.md)
- OpenSpec change: [`openspec/changes/establish-pa2-quality-gate`](openspec/changes/establish-pa2-quality-gate)

## License

Coursework repository for CSC13114 Advanced Web Application Development.
