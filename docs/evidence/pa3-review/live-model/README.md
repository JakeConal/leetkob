# Deployed NVIDIA NIM walkthrough

On November 28, 2026, the owner privately installed a replacement credential and
authorized the existing Azure deployment update, three bounded synthetic model
cases, protected merge, tag replacement and ZIP. No coursework upload is authorized.
The agent checked only credential-file presence and mode/ownership, never its contents.

- Public application: <https://leetkob-pa3-21ae08a0.southeastasia.cloudapp.azure.com/live>
- Tested application revision: `b8a0b5304bc136bdeaaabdc799fe9715a050a62f`.
- [Uncut fallback recording](walkthrough.webm): **2:06.60**, VP9, 1440×900,
  within the five-minute limit. Encoded duration and browser wall-clock timestamps
  differ; neither has been substituted for the other.
- [Browser report](browser-report.json): actual lab/submission IDs, original
  evidence, student-safe projections, snapshots before latest-submission
  supersession, zero page/unexpected console errors and no mobile overflow.
- [Sanitized calls](sanitized-calls.json): three actual provider request IDs,
  exact sanitized requests, prompt hashes, tokens, elapsed time and configured cost.
  These contain published synthetic fixture code, not student identities,
  session values, hidden tests or credentials. General operational logs and
  student routes do not expose these prompts.
- [Runtime readback](runtime.txt): live mode, isolated analyzer, persistent
  USD 0.25 lab cap, three settled zero-cost reservations, request ceilings and
  the unauthenticated session boundary.

## Observed results

The TA authored the exact official `sort-merge` statement, both constraints and
deductions, and all five tests through the product UI. No database lab seed was used.

| Official source                             | Judge   | C1 hard rule                | C2 live semantic result                      |
| ------------------------------------------- | ------- | --------------------------- | -------------------------------------------- |
| `s01` compliant merge sort                  | AC, 5/5 | pass                        | pass                                         |
| `s04` library sort                          | AC, 5/5 | violation, original line 11 | violation, original line 11                  |
| `s09` injection comment plus insertion sort | AC, 5/5 | pass                        | violation, original lines 10, 11, 13, 14, 17 |

Only C2 appears in each outbound constraint list: C1 was decided without model
dispatch. The injection comment is absent from the transmitted source, with its
original lines left blank to retain evidence mapping. Each payload has exactly
`statement`, `constraints` and `source`; no deduction field is sent.

Three dispatches used **2,820 input + 481 output tokens**, with no repair or
provider failure. Configured free developer-tier cost was **USD 0.00**. This
does not imply a paid tier is free. Known prices, a USD 0.25 lab cap, 160 reserved
calls per lab and 1,000 per UTC day remain enforced; changing tier requires
verified prices.

The TA explicitly confirmed C1's configured 3-mark deduction on `s04`. After a
reload, the saved decision and its reason remained visible and no new action
was preselected. Student review reads and the other-class TA received concealed
404s. Student results contained only judge data. Publication remained disabled;
no marks were published.

The capture script used the existing Playwright 1.65.1 module and official
browser image; it had no model key, application environment or Docker socket.
Only the trusted deployed worker performed provider calls. The script asserted
all three semantic outcomes and exact original-line evidence; it did not use
labels to produce predictions. This is a recorded release walkthrough, not a
claim of a new deterministic unit/contract test suite.

The separate first full dev evaluation remains unchanged at **78.4% recall**,
zero wrongful deductions and 3.9% unsure. This successful three-case deployment
exercise does not replace that result or prove the later PA#4 quality target.
