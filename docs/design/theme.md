# Press proof

The visual system for every LeeTKOB web screen: the PA#2 home at `/` and the PA#3 workspace under `/live`. It replaces the earlier Swiss Minimalist system.

The tokens and rules live in code. This page explains the intent; when it and the code disagree, the code wins and this page is updated.

| Layer          | File                                                               | Owns                                                         |
| -------------- | ------------------------------------------------------------------ | ------------------------------------------------------------ |
| Base           | [`apps/web/app/live/live.css`](../../apps/web/app/live/live.css)   | Token names, spacing scale, component structure, breakpoints |
| Theme          | [`apps/web/app/live/theme.css`](../../apps/web/app/live/theme.css) | Press proof values and treatments, scoped to `.live-press`   |
| Shell and type | [`apps/web/app/layout.tsx`](../../apps/web/app/layout.tsx)         | Fonts, `.live-workspace.live-press` wrapper, shared shell    |

## Idea

Marks are proofed before they go to press. A teaching assistant reads submitted code the way a proofreader reads a galley: find the line, mark it, decide. The palette is the four process inks, and each ink has one job.

## Color

Every color is a token. Components never use a raw value.

| Ink            | Token                       | Job                                                                |
| -------------- | --------------------------- | ------------------------------------------------------------------ |
| Process cyan   | `--cyan`, `--rail`          | The masthead and current navigation item                           |
| Deep cyan      | `--accent`, `--accent-text` | Links, selection, focus                                            |
| Key black      | `--key`                     | Primary actions and body text                                      |
| Proof magenta  | `--magenta`, `--bad`        | Deductions, violations, the blocked publication band               |
| Process yellow | `--yellow`, `--code-hit`    | Evidence: highlighted source lines and the matching evidence chips |
| Green          | `--ok`                      | Passed checks and accepted judge results                           |

Neutrals (`--bg`, `--surface`, `--sunken`, `--border`, `--text`, `--muted`) are one cool family tinted toward cyan. Status always carries a word as well as a color.

## Type

- Archivo for all interface text; headings are heavy and close-set.
- JetBrains Mono for code, ids and test data, with programming ligatures turned off so `<=` and `++` show exactly what the student typed.
- Geist and Geist Mono are the fallbacks. Use no other families.

## Signature elements

- **Press control strip:** a thin four-ink bar under the cyan masthead.
- **Galley source:** submitted code prints on paper-white with a tinted line-number gutter. An evidence line gets a yellow highlighter across the line and its number in magenta, and the evidence chips above the code use the same yellow.
- **Crop marks:** the publication gate and the sign-in panel carry printer's crop marks: what must clear before marks go to press.

Spend boldness only on these. Everything else stays quiet.

## Shape, depth and motion

- Radius 4px for controls and panels; square-cornered status tags with an ink border.
- Depth comes from borders and surface tints; avoid soft drop shadows.
- Motion is state feedback only (about 180ms). The evidence highlight sweeps in once. Everything stops under `prefers-reduced-motion`.

## Layout

- The navigation is a top masthead; content is centered at up to 1240px.
- One primary action per view, in key black.
- The review queue groups findings by submission, then shows each constraint's verdict, evidence and decision side by side.

## Accessibility and responsive behavior

- Body text contrast at least 4.5:1; visible `:focus-visible` on every control.
- Touch targets at least 44px on phones.
- At 1080px the lab and finding layouts stack; at 860px the navigation wraps; at 640px tables become stacked rows and controls go full width. No horizontal page scroll at 375px.

## Verifying a change

Run the repository checks, then check the rendered screens at 375, 768 and 1440px. For a local walkthrough without the Go API, `apps/web/scripts/mock-pa3-api.mjs` serves preview data for the `/live` screens; it is a UI preview tool only and never evaluation evidence.
