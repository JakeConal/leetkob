# LeeTKOB Agent Rules

## Stack

- Node.js 22.13+ with ESM, strict TypeScript, and npm workspaces.
- NestJS on Fastify in `apps/api`; Next.js App Router, React, and TanStack Query in `apps/web`.
- In-memory adapters are temporary PA#2 boundaries. Keep domain logic independent of storage, authentication, NestJS, and HTTP.

## Commands

- Install: `npm ci`
- Lint: `npm run lint`
- Format check: `npm run format:check`
- Type check: `npm run typecheck`
- Tests: `npm test`
- Production builds: `npm run build`
- Security: `npm run security`

## Conventions

- Keep domain rules pure and inject repositories, clocks, and authentication boundaries.
- Return errors as RFC 9457 `application/problem+json` with documented absolute type URIs.
- Add or update contract tests before changing externally visible API behavior.
- Prefer React Server Components; keep TanStack Query and other browser state in small client boundaries.
- Follow `docs/design/theme.md` for UI tokens, grid, typography, accessibility, and responsive behavior.
- Use pull requests after `main` protection is enabled; never bypass a required check.

## Never

- Never expose a secret to submitted code or a sandbox, including through environment variables.
- Never make a real model call in unit or contract tests. See `AI-LOG.md` decision D1.
- Never edit a test, eval case, expected label, or contract merely to make a check pass.
- Never trust caller-supplied role headers; verify signed identity claims. See `AI-LOG.md` decision D2.
- Never publish a lab while any review remains pending.

## Permissions

### Allowed

- Read repository files and run documented checks.
- Edit implementation, tests, contracts, and documentation within the requested change.
- Create local commits and push feature branches for the requested work.

### Ask first

- Add another runtime dependency or change the documented API contract.
- Change repository visibility, branch protection, CI permissions, or deployment settings outside PA#2.
- Trigger a paid model request, external deployment, or destructive migration.

### Never

- Read, print, commit, transmit, or rotate a live credential.
- Weaken or delete a required test, eval case, label, or branch check to obtain green CI.
- Submit coursework, post messages, or act as a team member without the user's explicit instruction.
