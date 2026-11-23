---
name: openspec-distribute-tasks
description: Split an OpenSpec change's GitHub checklist into one issue per task, distribute tasks equally among confirmed team members, and link native GitHub blocking dependencies. Use for requests to divide, assign, or distribute OpenSpec tasks across the team.
---

# Distribute OpenSpec Tasks

Create one assigned GitHub sub-issue per numbered task in an existing change's tracking issue. Preserve the parent checklist and attach native blocking relationships between prerequisites and dependent tasks.

Creating or editing this skill authorizes skill files only. Execute distribution when the user requests task distribution or invokes this skill for that purpose. A request to preview produces a proposed allocation without GitHub mutations. Distribution covers task issues, assignments, parent links, and dependencies. Implementation starts through the apply workflow.

## Resolve the change and team

1. Infer the change from the request or current conversation. Otherwise run `openspec list --json` and select the sole active change or ask which change to use. Use `openspec.cmd` on Windows when PowerShell blocks the script shim.
2. Run `openspec context --json` and `openspec status --change <change> --json`. Use the returned root and changeRoot. If a registered store is explicitly selected, resolve its ID with `openspec store list --json` and keep `--store <id>` on supported commands. Stop on unresolved roots or stores without creating planning directories.
3. Resolve the implementation repository and GitHub hostname from its Git remote. Always pass `--repo <owner/repo>` to `gh issue` commands and the resolved hostname to API commands when needed. A standalone specification store does not determine the implementation repository.
4. Read `.openspec.yaml` at changeRoot and retrieve its `x-github-issue` with `gh issue view <parent> --repo <repo> --json number,title,state,body,url`. Verify the issue belongs to this change and contains a nonempty numbered checklist. Read the proposal, design, and relevant specs. An absent local task file is expected in this project. Never create or rely on `tasks.md`.
5. Get GitHub usernames from the user's roster or explicit project team records. Collaborators, commit authors, upstream proposal authors, and R1/R2/R3 placeholders alone do not identify the intended members. If no verified roster exists, ask for the member usernames before assigning. Verify each username is assignable using repository assignee APIs. Do not invite members or change repository access.

## Identify tasks and reuse existing issues

Read numbered checkboxes of the form `- [ ] N.N Description` or `- [x] N.N Description`. A checkbox containing only x/X with surrounding whitespace is complete. Every other marker is incomplete. Reject duplicate task IDs and ambiguous checklist boundaries before creating issues. Preserve task IDs, wording, acceptance criteria, ordering, and existing checkbox state.

Use the tuple `(repository, parent issue, change name, task ID)` as task identity. Paginate all reads. Inspect existing linked sub-issues, parent task links, and exact-title-prefix search results in open and closed issues. Search results are candidates and require a matching body marker. Before creating each issue, repeat the identity lookup. Reuse a unique matching issue. Stop on duplicate matches or conflicting task identities.

### Naming convention

Use this title format for every task issue.

```text
[<change-name>] <task-id> <imperative summary>
```

Example titles follow the stable IDs from the source checklist.

```text
[complete-leetkob-product-workflow] 2.2 Add account sessions
[complete-leetkob-product-workflow] 4.1 Record submission receipt
```

Keep the original unpadded task ID, such as `4.1` or `10.3`. Write a concise verb-led summary describing the deliverable. Keep the change/task prefix immutable when editing the summary. Ownership and blocking state belong in assignments and dependency relationships.

Each issue body must include a stable identity marker, the parent link, the original task text, required deliverables, acceptance/verification criteria, relevant spec/design paths, and prerequisite task IDs with a short rationale. Use ordinary acceptance bullets so the parent remains the single numbered completion checklist. Example body structure follows.

```markdown
<!-- openspec-task change=complete-leetkob-product-workflow parent=7 task=4.1 -->

## Source

Parent issue #7, task 4.1 in change complete-leetkob-product-workflow.

## Task

Copy the complete source task text here, including its verification requirement.

## Acceptance

- Describe the observable completion and verification criteria from the source.

## Planning references

- Relevant capability spec and design section.

## Prerequisites

- Task 2.1 supplies the durable admission storage required by this task.

## Completion tracking

The parent task checkbox is authoritative. Close this task issue as completed only after its full acceptance criteria are verified and the matching parent checkbox is checked. A closed issue alone does not prove completion.
```

## Balance assignments

Default to equal task counts. With T tasks and M confirmed members, every member receives floor(T/M) or ceil(T/M) tasks. For example, 70 tasks across three members yields 24, 23, and 23. Include all source tasks in the issue mapping. Preserve completed work and its verified historical owner, and balance remaining work separately.

Order unfinished tasks by phase and dependency order. Use a rotating member order to distribute each phase, filling the members with the fewest allocated unfinished tasks first. Break count ties by lower total estimated effort, then by stable username order. Estimate effort as small/medium/large using weights 1/2/3 from the actual acceptance scope. Label estimates as assumptions. Honor user-specified availability or expertise constraints and disclose any resulting count difference. Equal count is the default measurable invariant, with estimated effort as a secondary balancing signal.

On reruns, retain existing assignments and assign only undistributed tasks. Do not silently reassign another member's issue. If fixed historical assignments or explicit constraints prevent the requested balance, report the imbalance and ask before rebalancing existing work. Give every issue one accountable assignee. A task's dependency does not require the same owner.

## Determine prerequisites

Build a directed acyclic graph from explicit task prerequisites and concrete deliverables required by later tasks. Record a reason for each inferred edge in the allocation plan. Ask about ambiguous prerequisites that materially affect who can start. Number order, a shared phase, or a common assignee alone does not imply a dependency. Avoid making every later task wait for an entire earlier phase when only one deliverable is required.

Reject cycles, self-dependencies, and unknown prerequisite IDs. Retain existing confirmed external dependencies. Show the allocation table with task ID, proposed title, assignee, estimated effort, and prerequisite IDs before mutation. If distribution is already authorized and the roster and dependencies are resolved, continue without another approval step.

## Create issues and native relationships

Use connected GitHub capabilities when they support the required operations. Otherwise use authenticated `gh` commands with temporary UTF-8 body files. Keep credentials in the existing authenticated client. Creation examples use the resolved repository and real usernames.

```text
gh issue create --repo <repo> --title "[<change>] <task-id> <summary>" --body-file <temporary-file> --assignee <username>
```

Create or reuse all child issues before adding dependency edges. After each creation, read the issue back and verify its marker and assignment. Capture both the issue number and its numeric database ID through `gh api repos/<repo>/issues/<number>`. The database ID is required for relationship payloads.

Attach each child to the parent using native sub-issues after checking its current parent. A child already attached to a different parent requires user guidance before moving it.

```text
gh api --method POST repos/<repo>/issues/<parent-number>/sub_issues -F sub_issue_id=<child-database-id>
```

For dependent task B requiring prerequisite A, add A to B's native blocked-by relationships. Issue number and database ID are different values.

```text
gh api --method POST repos/<repo>/issues/<B-number>/dependencies/blocked_by -F issue_id=<A-database-id>
gh api --paginate repos/<repo>/issues/<B-number>/dependencies/blocked_by
```

Read existing relationships first and create only missing edges. Use GitHub's native blocked-by state. Never create, add, or depend on a `Blocked` label, including spelling/case variants. A textual dependency list supplements the native relationship. Verify prerequisites that are closed were completed, rather than cancelled, before reporting a dependent task as ready. If a closed prerequisite lacks verified completion, report that conflict and leave the dependent out of the ready list.

Native API permission or feature failures are blockers to this workflow's dependency requirement. Report the specific failed operation and affected issues. Preserve successfully created issues and links for resumption. Do not substitute a label or claim a text-only link establishes the native relationship. After timeouts or ambiguous create responses, search/read back before retrying. Honor rate-limit retry times and stop on persistent errors with an inventory of completed and outstanding operations.

API references explain the supported relationship operations. Check the installed host's support if it differs from GitHub.com.

- [GitHub issue dependencies](https://docs.github.com/en/rest/issues/issue-dependencies)
- [GitHub sub-issues](https://docs.github.com/en/rest/issues/sub-issues)

## Preserve parent tracking and verify

Keep `x-github-issue` pointing to the original parent. Add each child link and assignee as a suffix on its matching parent checklist line while retaining the leading checkbox, task ID, and original task wording. For example, append ` | Task issue #42 | @member`. Preserve unrelated body content. Re-read the parent before editing, reconcile changes to task lines, and abort conflicting edits. Use `gh issue edit <parent> --repo <repo> --body-file <temporary-file>`, then read back the full checklist.

Verify every source task has exactly one corresponding child, every unfinished child has the intended assignee, the parent-child links exist, and every planned prerequisite is present in the native blocked-by response. Confirm the unfinished assignment counts differ by at most one unless an explicit constraint was reported. Existing checked tasks require validated completion evidence before closing a new child as completed. Neither distribution nor a child closure changes an unchecked parent task to complete. During subsequent implementation, verify the task, update its exact parent checkbox, and then close its child issue. Read native prerequisites and verified parent completion before starting dependent work.

Report the parent URL, created/reused issues, per-member counts and estimated effort, ready and blocked tasks with prerequisite links, and any partial failures or unresolved state conflicts. Mention that estimates and inferred dependencies were proposed during allocation. Do not close the parent or mark implementation complete as part of distribution.
