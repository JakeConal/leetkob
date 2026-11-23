# PA#3 live screens

The signed-in workspace is at `/live`. The PA#2 home page stays at `/`. Browser calls use `fetch` with `credentials: "include"` against same-origin `/v2`. Next.js proxies those paths to `PA3_API_ORIGIN` (default `http://127.0.0.1:8080`). Mutations send `X-CSRF-Token` from the login response and an `Idempotency-Key`. Protected responses stay in memory and are removed on logout. A failed connection shows the connection error and does not invent a judged submission or a created lab.

## Routes

| Screen            | Route                              | API                                                                                                          |
| ----------------- | ---------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| Sign in           | `/live`                            | `POST /v2/auth/login`, `GET /v2/auth/me`                                                                     |
| Log out           | navigation on every live screen    | `POST /v2/auth/logout`                                                                                       |
| Classes           | `/live/classes`                    | `GET /v2/auth/me`, `GET /v2/classes`                                                                         |
| Class labs        | `/live/classes/{classId}`          | `GET /v2/auth/me`, `GET /v2/classes/{classId}/labs`                                                          |
| Create lab        | `/live/classes/{classId}/labs/new` | `POST /v2/classes/{classId}/labs`                                                                            |
| Lab               | `/live/labs/{labId}`               | `GET /v2/labs/{labId}`, `GET /v2/auth/me`, `POST /v2/labs/{labId}/submissions`                               |
| Submission status | `/live/submissions/{submissionId}` | `GET /v2/submissions/{submissionId}`                                                                         |
| TA review         | `/live/labs/{labId}/reviews`       | `GET /v2/labs/{labId}/reviews`, `GET /v2/labs/{labId}`, `POST /v2/labs/{labId}/reviews/{findingId}/decision` |
| Concealed 404     | `/live/not-found`                  | none                                                                                                         |

A `404` from a lab, class, submission, or review request replaces the screen with Not found. That page does not render class, source, or review data.

## Authoring walkthrough

1. Start the Go API on port 8080 and the web app (`npm run dev:web` from the repository root). Set `PA3_API_ORIGIN` if the API is not on `http://127.0.0.1:8080`. The API web origin must be the Next origin so cookie and CSRF checks accept the browser.
2. Open `http://127.0.0.1:3000/live` and sign in as the assigned TA. A wrong password stays on the form and shows the problem `detail`.
3. Open the class, then Create lab. Enter the statement, sample tests, hidden tests, two constraints with deductions, the opening time, the deadline, CPU seconds, and memory. Submit.
4. If the API rejects the body, the form stays and shows the problem `detail`. No lab id is shown for that attempt.
5. Log out. The class and lab data leave the screen. Sign in as an enrolled student, open the lab, and confirm the model-review notice and sample tests. Hidden test contents are not on the page.
6. Submit source. The status page polls `GET /v2/submissions/{submissionId}` and shows judge state and test timing only.
7. Log out, sign in as the assigned TA, and open Review. Each finding shows the checker verdict, evidence lines, submitted source, and a deduction choice of pass, violation, or unsure. Unsure starts unselected and does not send a confirming decision. Publish lab stays disabled and says publication is blocked.
8. Open another class's lab or queue. The API's concealed 404 becomes the not-found page.
