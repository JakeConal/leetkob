# LeeTKOB web workspace (PA#3)

Next.js App Router front end for the PA#3 checkpoint. It is a deliberately
small client over the Go API in [`apps/backend`](../backend): sign-in, the
current account's classes, assigned-TA lab authoring, and a safe lab view.

## Routes

| Screen        | Path                          | Data source                          |
| ------------- | ----------------------------- | ------------------------------------ |
| Sign in       | `/sign-in`                    | `POST /v2/auth/login`                |
| Classes       | `/classes`                    | `GET /v2/auth/me`, `GET /v2/classes` |
| Lab authoring | `/classes/{classId}/labs/new` | `POST /v2/classes/{classId}/labs`    |
| Lab detail    | `/labs/{labId}`               | `GET /v2/labs/{labId}`               |

Browser code never calls the Go origin directly. `app/v2/[...path]/route.ts`
proxies an allowlist of `/v2` routes to `GO_API_URL` (default
`http://127.0.0.1:8080`), forwards the `Cookie`, `Origin`, `X-CSRF-Token` and
`Idempotency-Key` headers, and returns every response with `Cache-Control:
no-store`. The opaque session cookie is therefore same-origin and HttpOnly:
JavaScript can neither read it nor send it anywhere else.

Session state is in memory only (`lib/protected-state.ts`). The CSRF proof and
any cached protected reads disappear on sign-out, on a 401, or when
`/v2/auth/me` reports a different account. Nothing is written to `localStorage`
or `sessionStorage`. A full page reload keeps the cookie but loses the CSRF
proof, so the class screen asks you to sign in again before a mutation.

## Authoring walkthrough (real API, no seed workaround)

Prerequisites: Node.js 22.13+, Go, and Docker for the pinned local PostgreSQL.
See [`apps/backend/README.md`](../backend/README.md) for the full database and
API details.

1. Start the loopback database and export the connection string:

   ```bash
   docker run -d --name leetkob-pg \
     -e POSTGRES_PASSWORD=leetkob-dev \
     -p 127.0.0.1:5432:5432 \
     postgres@sha256:eb63372c1e05c755f55fa30cbd07aecc443676fc6c0fa90b9002d01537841e1a

   export DATABASE_URL='postgres://postgres:leetkob-dev@127.0.0.1:5432/postgres?sslmode=disable'
   ```

2. Provision the five synthetic accounts (idempotent; never creates the marked
   lab). Record the passwords printed once:

   ```bash
   cd apps/backend
   DATABASE_URL="$DATABASE_URL" go run ./cmd/provision
   ```

3. Start the Go API on its loopback default (`:8080`) and the web app:

   ```bash
   # terminal 1, from apps/backend
   DATABASE_URL="$DATABASE_URL" go run ./cmd/api

   # terminal 2, from the repository root
   npm run dev:web
   ```

   `GO_API_URL` defaults to `http://127.0.0.1:8080`. Set it if the API listens
   elsewhere. The API's `WEB_ORIGIN` must match the browser origin
   (`http://127.0.0.1:3000` by default).

4. Open `http://127.0.0.1:3000/sign-in` and sign in as `ta1@leetkob.local` with
   the password from step 2.

5. On `/classes`, confirm the account name, `ta` role and the `PA3 Class A`
   membership, then choose **Create lab**.

6. Fill the form and submit with **Create lab**:

   - **Title** and **Statement**: 1-200 characters and up to 64 KiB.
   - **Opens at** / **Deadline**: local date-times, converted to RFC 3339 UTC;
     the deadline must be strictly after the opening time.
   - **Constraints**, one per line as `ID | text | deduction`, for example:

     ```text
     C1 | Do not call `std::sort`. | 3
     C2 | Implement merge sort. | 4
     ```

     1-20 unique IDs matching `[A-Za-z][A-Za-z0-9_-]{0,31}`, text 1-4096
     characters, deductions 0-10 with at most two decimals. Ban-list texts
     start with "Do not use" or "Do not call"; backticks name the banned
     symbol.

   - **Tests** as a JSON array of
     `{ "id", "input", "expectedOutput", "sample" }`, 1-500 unique IDs and at
     most 10 MiB of combined input/output. Only `sample: true` tests are visible
     in the lab view.
   - **Limits**: CPU 1-5 seconds and memory 64-512 MiB.

   The screen validates the same bounds, sends the in-memory `X-CSRF-Token`
   plus a random 32-hex-character `Idempotency-Key`, and shows the created lab
   revision (`1`). A retry reuses the same key until a request succeeds, so a
   repeat cannot create a second lab.

7. Choose **Open lab** to read `/labs/{labId}`. The page shows the statement,
   constraints with TA-configured deductions, limits, the latest revision, the
   API's model notice, and sample tests only. Hidden tests and checker findings
   are never serialized.

## Verification commands

```bash
npm run typecheck --workspace @leetkob/web
npm run test --workspace @leetkob/web
npm run build --workspace @leetkob/web
npx eslint apps/web scripts/openapi-drift-check.mjs
npx prettier --check "apps/web/**" scripts/openapi-drift-check.mjs
```

The client-logic tests are network-free and cover problem-detail parsing, CSRF
and idempotency header injection, 401 cache clearing, account-switch clearing,
lab-form bounds, the proxy route allowlist, and contract drift (positive plus a
mutated `/tmp` fixture).

`apps/web/lib/api-types.ts` is hand-derived from
`docs/contract/pa3-openapi.json`. Re-run the drift check whenever the contract
changes:

```bash
node scripts/openapi-drift-check.mjs
```

The repository owner can register it as:

```json
"check:openapi-drift": "node scripts/openapi-drift-check.mjs"
```

## Manual browser verification still required

The automated suites cannot assess rendering. Before marking PA#3 UI work
complete, a reviewer must verify in a real browser:

- keyboard-only sign-in, class list, lab form and lab detail: logical Tab
  order, every control reachable, visible focus outline, and no focus trap;
- narrow viewport (at or below 700px): the primary column, forms, facts,
  constraint table and sample tests stack in DOM reading order, the source and
  table scroll inside their own regions, and display text does not collide with
  controls;
- the model notice and sample-only wording are visible in the lab view;
- sign-out followed by a different account shows no previous account data.

Screenshots and a keyboard walkthrough at narrow width were not captured by the
implementation session.
