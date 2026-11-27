# Swiss Minimalist practice workspace

The practice workspace uses Host Grotesk, white and pale-grey surfaces, black rules, square controls, and small red signals. Lab browsing and the description/editor split follow a coding-practice workflow. All existing `/live` assessment routes share this shell.

## Implementation

| Layer                          | File                                      | Responsibility                                                                 |
| ------------------------------ | ----------------------------------------- | ------------------------------------------------------------------------------ |
| Shared tokens and components   | `apps/web/app/globals.css`                | Host Grotesk, navigation, forms, catalog, split editor, result-release preview |
| Assessment component structure | `apps/web/app/live/live.css`              | Existing authoring, review, class and submission component structure           |
| Assessment theme               | `apps/web/app/live/practice.css`          | Maps assessment tokens to the shared Swiss palette and square geometry         |
| Shared shell                   | `apps/web/components/app-shell.tsx`       | Masthead, persistent workspace index, reading order and footer                 |
| Session boundary               | `apps/web/components/live/live-shell.tsx` | Existing sign-in, sign-out and protected-query clearing                        |

`apps/web/app/live/theme.css` retains the earlier Press proof theme as an unimported historical implementation. It is not layered over the practice theme.

## Typography and palette

Host Grotesk is bundled locally in `apps/web/public/fonts/` with its SIL Open Font License and bound to `SwissMinimalistHost`. Use heavy, tightly tracked display headings, readable body text, and explicit control sizes. Monospace is reserved for source code and structured data.

| Token                  | Value                                      | Use                                             |
| ---------------------- | ------------------------------------------ | ----------------------------------------------- |
| `--dp-color-ground`    | `#ffffff`                                  | Main canvas                                     |
| `--dp-color-ink`       | `#000000`                                  | Text, strong rules and primary actions          |
| `--dp-color-surface`   | `#f0f0f0`                                  | Supporting surfaces                             |
| `--dp-color-accent`    | `#c90000`                                  | Selected emphasis, errors and attention markers |
| `--dp-typography-face` | `"SwissMinimalistHost", Arial, sans-serif` | Interface typography                            |
| `--dp-radius-control`  | `0`                                        | Square controls                                 |
| `--dp-border-rule`     | `2px`                                      | Strong dividing rules                           |
| `--dp-motion-feedback` | `0s`                                       | Immediate state feedback                        |

Avoid floating cards, shadows, gradients, translucent layers, ornamental backgrounds and decorative fonts. Status must carry readable text as well as a marker or color.

## Layout and behavior

- Keep the masthead and persistent index rail aligned with the content. Use continuous rules and deliberate whitespace instead of nested cards.
- Browse labs in a searchable table, filtered by class and scheduled availability. Availability is informational; the API determines submission eligibility.
- On wider screens, show the description, constraints and samples beside a line-numbered C++17 editor. Reuse the existing submission projection and polling for inline results and the dedicated submission route.
- At narrower widths, stack the problem and editor, turn the index into navigation above the content, and keep verdicts and timings readable without page overflow.
- Keep keyboard focus visible, preserve reading order, label every form field and honor reduced-motion preferences. Never trap Tab inside the editor.
- The result-release page is explicitly a workflow preview. Separate example readiness, outstanding example reviews, student visibility and navigation to real labs. Production release stays disabled; completing reviews never publishes marks automatically.

## Functional boundaries

The redesign keeps the existing API, session, authorization, hidden-test projections, TA review decisions and production publication gate. It adds no runtime dependency or model call. Tests and evaluation labels remain authoritative.
