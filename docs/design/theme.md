<prompt>
<task>
Apply this visual style to the user's existing project. Inspect and preserve its stack, copy, routes, and behavior. Follow existing component and styling conventions. Treat framework, library, and page examples in these design instructions as illustrations; they do not require a stack change or new sections. Scope changes to the user's requested work.

Keep controls keyboard accessible, focus visible, and text legible. Preserve reading order, responsive behavior, and reduced-motion preferences. Verify actual states at the rendered width. The user's stack and content take precedence over illustrative implementation details.
</task>

<design-system>
# Swiss Minimalist

## Identity

Use a continuous typographic grid in white, black, and pale grey, with small red signals. Large heavy headings, exposed rules, and a persistent index rail carry the composition. Keep content aligned across sections so the page reads as one organized document.

## Layout

Start with a twelve-column desktop grid. Give a broad opening statement about nine columns and a narrow supporting or action area the remainder. In later sections, reserve the first three columns for an index and section heading; begin the main reading and content matrix on the same fourth-column line.

Continue this rail through text, capabilities, and editorial images. Related steps and benefits can share a full black field with white reading. Comparisons can use equal ruled columns, and a closing statement can echo the opening's broad heading beside a narrow action area. Finish with a fully divided footer aligned to the same grid.

Choose the patterns that fit the brief. Their relationships matter more than a fixed number of sections, entries, or columns of sample content.

## Typography

Use Host Grotesk throughout, bound to "SwissMinimalistHost". Make display text heavy, tightly tracked, and close-set. The opening scale runs from 64px to 148px on wider layouts, with line height below one. Smaller headings remain forceful; paragraphs use a lighter weight and enough leading for sustained reading. Use plain small labels and numbers to make the index useful. Do not add a second decorative typeface.

## Color and surfaces

White is the main ground; black supplies ink, rules, and occasional full-width fields. Pale grey distinguishes a small amount of supporting content. Red is reserved for compact markers, featured labels, and selected emphasis.

Keep corners square and dividing rules visible. Content should connect through edges and shared alignment. Avoid floating cards, shadows, translucent layers, and ornamental backgrounds.

## Components

Use rectangular controls with direct black-and-white states and a clearly visible focus outline. Small red marks can identify priority without becoming the only state cue.

Features form a ruled matrix. Numbered steps share a continuous field instead of separate cards. If comparisons are needed, keep their columns equal and use a compact red flag for the featured option. Selection must have its own readable state. Disclosures use strong horizontal rules and a simple open/close marker.

## Images

Use grayscale editorial photographs in simple rectangular crops. A desktop image series can use tall proportions and align its captions to the shared grid. Repeating a subject is optional; choose imagery that explains the customer's content. Keep details legible without color and use alternative text for meaningful subjects.

## Motion

Use immediate visual feedback. The design relies on type, alignment, and contrast rather than animated entrances, image drift, or eased control movement.

## Responsive behavior

At 1050px, editorial items can become image-and-text rows and numbered steps become a vertical sequence. At 700px, place the index rail above its content, stack the major grids and comparisons, and turn facts into readable rows. Change tall image crops to wider proportions where needed. Keep the rules and black field continuous, with smaller gutters and two-column footer groups.

## Design limits

Preserve a clear hierarchy even when headings are large. Do not let display text collide with controls or run offscreen. Avoid decorative geometry, gradients, excessive red, and artificial content added to fill the grid. The structure must adapt to the actual information.

## Authoritative tokens

| CSS variable                   | Value                                    |
| ------------------------------ | ---------------------------------------- |
| --dp-color-ground              | #ffffff                                  |
| --dp-color-ink                 | #000000                                  |
| --dp-color-surface             | #f0f0f0                                  |
| --dp-color-accent              | #c90000                                  |
| --dp-typography-face           | "SwissMinimalistHost", Arial, sans-serif |
| --dp-typography-body           | 16px                                     |
| --dp-typography-label          | 12px                                     |
| --dp-typography-metadata       | 14px                                     |
| --dp-typography-lead           | 17px                                     |
| --dp-typography-item-title     | 24px                                     |
| --dp-typography-regular        | 400                                      |
| --dp-typography-strong         | 700                                      |
| --dp-typography-display        | 750                                      |
| --dp-typography-label-tracking | 0.12em                                   |
| --dp-typography-body-leading   | 1.5                                      |
| --dp-spacing-gutter            | clamp(20px, 3.3333cqw, 64px)             |
| --dp-radius-control            | 0                                        |
| --dp-border-rule               | 2px                                      |
| --dp-motion-feedback           | 0s                                       |

</design-system>
</prompt>
