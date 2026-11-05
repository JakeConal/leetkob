---
name: openspec-continue-change
description: Continue working on an OpenSpec change by creating the next artifact. Use when the user wants to progress their change, create the next artifact, or continue their workflow. Also use when the user says "openspec continue" or "opsx continue".
allowed-tools: Bash(openspec:*)
license: MIT
compatibility: Requires openspec CLI.
metadata:
  author: openspec
  version: "1.0"
  generatedBy: "1.14.3"
---

Continue working on a change by creating the next artifact.

**Store selection:** If the user names a store (a store is a standalone OpenSpec repo registered on this machine) or the work lives in one, run `openspec store list --json` to discover registered store ids, then pass `--store <id>` on the commands that read or write specs and changes (`new change`, `status`, `instructions`, `list`, `show`, `validate`, `archive`, `doctor`, `context`, `schemas`, `view`). Once selected, treat `--store <id>` as sticky for the rest of the workflow. Every unscoped example of those commands below is shorthand: before running it, append the flag. For example, run `openspec status --change "<name>" --json --store "<id>"`, not the unscoped form shown below. Other commands do not take the flag. Hints printed by commands already carry the flag; keep it on follow-ups. Without a store, commands act on the nearest local `openspec/` root.

**Project check:** These steps expect a project that already uses OpenSpec. Before the first step that writes anything (`new change`, `archive`, `sync specs`, or authoring an artifact file), confirm the project has a root: run `openspec list --json` (with `--store <id>` when a store is selected, since the store is then the root) and read `root`. A root object means the project is set up. `"root": null` means it is not - there is no `openspec/` directory here, and a write such as `openspec new change` would create one as a side effect. The command also exits non-zero, which is that answer rather than a broken CLI, so read the JSON instead of retrying or working around it.

One `"root": null` is not about setup: when a `status` error message starts with `Declared in` or `Invalid store declaration in` and names this project's `openspec/config.yaml` (or `config.yml`), the project does use OpenSpec through a store it declares, which this machine cannot resolve (the store is not registered, or the `store:` line is malformed). Do not treat it as uninitialized and skip the branches below: stop before writing and show the user that error's `message` and `fix`.

Otherwise, with no root, what happens next depends on how this workflow was reached:

- **Auto-selected**: you chose this workflow yourself, without the user naming OpenSpec, naming this skill, or running its slash command. Stop using OpenSpec and answer the request normally, as you would with no OpenSpec installed. Do not ask them to set anything up and do not mention OpenSpec setup.
- **Explicit OpenSpec request**: the user named OpenSpec, named this skill, or ran its slash command. Stop before writing and ask how to proceed: set this project up (`openspec init`), target a store they already have (`--store <id>`), or continue without OpenSpec for this request. Wait for their answer.

In both branches, never create the root as a side effect: do not run `openspec init` until the user asks for it, do not hand-create `openspec/` files, and do not let a command create it.

**GitHub issue task tracking:** This project replaces the `tasks.md` artifact with one GitHub issue per change.

- Resolve the target GitHub repository from the implementation repository remote, not a standalone spec store. Add `--repo <owner/repo>` to every `gh issue` command below.
- Reuse the recorded issue. After interrupted creation, search the target repository for the exact `[<change-name>]` title prefix and confirm the matching change before creating another issue. If lookup or creation fails, report the blocker without falling back to a task file.
- A recorded number satisfies the task artifact only after verifying the issue exists in the target repository and contains a nonempty numbered task checklist. Preserve unrelated metadata and issue body content. A closed issue alone never proves task completion.
- Record the issue number in the change's `.openspec.yaml` as `x-github-issue: <number>`. OpenSpec keeps `x-*` extension keys, so the CLI and archive preserve it.
- The issue body is the single source of truth for the task checklist. Use the tracked format OpenSpec expects: `- [ ] N.N Task description`; a box holding only `x`/`X` is complete and every other marker is incomplete.
- Never create, edit, or rely on `tasks.md`. When status or apply instructions report the `tasks` artifact missing, or `state: "blocked"`/`"no-tasks"` only because of `tasks`, use the GitHub issue instead and continue.
- Read with `gh issue view <number> --json number,title,state,body,url`. Update checkboxes only by editing the issue body with `gh issue edit <number> --body-file <file>`; never track completion in a comment.
- Create with `gh issue create --title "[<change-name>] <summary>" --body-file <file>`, then record the returned issue number in `.openspec.yaml`.

**Input**: Optionally specify a change name. If omitted, check if it can be inferred from conversation context. If vague or ambiguous you MUST prompt for available changes.

**Steps**

1. **Select the change**

   If a name is provided, use it. Otherwise:
   - Infer from conversation context if the user mentioned a change
   - Auto-select if only one active change exists
   - If ambiguous, run `openspec list --json` to get available changes sorted by most recently modified, and ask the user to select one

   When prompting, present the top 3-4 most recently modified changes as options, showing:
   - Change name
   - Status (e.g., "0/5 tasks", "complete", "no tasks"), counting tasks from the change's GitHub issue when `x-github-issue` is recorded
   - How recently it was modified (from `lastModified` field)

   Mark the most recently modified change as "(Recommended)" since it's likely what the user wants to continue.

   Always announce: "Using change: <name>" and how to override (e.g., `/opsx-continue <other>`).

2. **Check current status**

   ```bash
   openspec status --change "<name>" --json
   ```

   Parse the JSON to understand current state. The response includes:
   - `schemaName`: The workflow schema being used (e.g., "spec-driven")
   - `artifacts`: Array of artifacts with their status ("done", "skipped", "ready", "blocked")
   - `isPlanningComplete`: Boolean indicating if all planning artifacts are complete. Older CLI versions expose the same value as `isComplete`.
   - `planningHome`, `changeRoot`, `artifactPaths`, and `actionContext`: path and scope context. Use these instead of assuming repo-local paths.

3. **Act based on status**:

   ***

   **Before anything else, finish a partly written artifact**:
   - An artifact whose output is a glob pattern (e.g. `specs/**/*.md`) reads `done` as soon as one matching file exists, so a run interrupted while writing its files leaves it `done` with some still missing
   - For each such `done` artifact, first get its instructions and read any completed dependencies needed to determine the expected files. Then compare those files with `artifactPaths.<id>.existingOutputPaths`. For spec-driven's `specs`, read the proposal and check for one `specs/<capability-path>/spec.md` per capability it lists; resolve each expected path against `changeRoot` before comparing path identity, and verify each one remains inside `changeRoot` after resolving symlinks, both in its parent directories and in the file itself if it already exists
   - Never write an expected path that fails that check; stop and tell the user. If any are missing, write only the missing files, each at its checked path - when the `instruction` field delegates creation to a skill or command, invoke it only if it can write each file to its checked path; otherwise stop and tell the user - and STOP - that counts as this invocation's ONE artifact. If you cannot tell whether a file was left out on purpose, ask the user

   ***

   **If all planning artifacts are complete (`isPlanningComplete: true`, or legacy `isComplete: true`)**:
   - Congratulate the user
   - Show final status including the schema used
   - Suggest: "Planning is complete! You can now implement this change. Once implementation and any tracked work are complete, archive it."
   - STOP

   Treat planning as complete when the only remaining artifact is `tasks` and the change records `x-github-issue` pointing to a verified issue with a nonempty task checklist; the CLI keeps reporting `tasks` as `ready` because it looks for `tasks.md`.

   ***

   **If artifacts are ready to create** (status shows artifacts with `status: "ready"`):
   - Pick the FIRST artifact with `status: "ready"` from the status output
   - If that artifact is the schema's `tasks` artifact (its `outputPath` is `tasks.md`), use its instructions for the checklist format, but do not write a file: draft the task checklist, create the GitHub issue as described in **GitHub issue task tracking**, record its number in `.openspec.yaml` as `x-github-issue`, show what was created and what's now unlocked, and STOP after creating this ONE artifact
   - Get its instructions:
     ```bash
     openspec instructions <artifact-id> --change "<name>" --json
     ```
   - Parse the JSON. The key fields are:
     - `context`: Project background (constraints for you - do NOT include in output)
     - `rules`: Artifact-specific rules (constraints for you - do NOT include in output)
     - `template`: The structure to use for your output file
     - `instruction`: Schema-specific guidance
     - `resolvedOutputPath`: Resolved path or pattern to write the artifact
     - `dependencies`: Completed artifacts to read for context (entries with `skipped: true` have no files - do not look for them)
     - `skipped`/`warning`: present when the change declares skip_specs and this artifact must NOT be created - pick another artifact
   - **Create the artifact file**:
     - Read any completed dependency files for context - always re-read them from disk, even if you saw them earlier in the conversation (the user may have edited them)
     - Choose the output path before creating anything. If the `resolvedOutputPath` specified in instructions is a glob pattern, choose a concrete path inside `changeRoot` using the schema instruction and the change's context, and verify it remains inside `changeRoot` after resolving symlinks, both in its parent directories and in the file itself if it already exists; if it does not, stop and tell the user. Otherwise the output path is `resolvedOutputPath`
     - If the `instruction` field delegates creation to a specific skill or command, invoke it only if it can write the artifact to that output path; otherwise stop and tell the user
     - Otherwise, write it yourself:
       - Use `template` as the structure - fill in its sections
       - Apply `context` and `rules` as constraints when writing - but do NOT copy them into the file
       - Write the file to that output path
     - Either way, verify the artifact file exists at that output path
   - Show what was created and what's now unlocked
   - STOP after creating ONE artifact

   ***

   **If no artifacts are ready (all blocked)**:
   - This shouldn't happen with a valid schema
   - Show status and suggest checking for issues

4. **After creating an artifact, show progress**
   ```bash
   openspec status --change "<name>"
   ```
   When the artifact was `tasks`, show the GitHub issue number, URL, and checkbox progress alongside the CLI status, which still lists `tasks` as `ready`.

**Output**

After each invocation, show:

- Which artifact was created
- Schema workflow being used
- Current progress (N/M complete)
- What artifacts are now unlocked
- Prompt: "Want to continue? Just ask me to continue or tell me what to do next."

**Artifact Creation Guidelines**

The artifact types and their purpose depend on the schema. The `instruction` field from the instructions output is the authoritative guidance for each artifact - follow it even when the artifact has a familiar name (proposal.md, design.md, etc.), since custom schemas may define different content or a different process for the same file names. The schema's `tasks` artifact is the one exception: its output is a GitHub issue, never `tasks.md`.

If the `instruction` field directs you to use a specific skill or command to create the artifact, invoke it instead of writing the artifact directly - but only once you have chosen and checked the output path, and only if it can write there.

**Guardrails**

- Create ONE artifact per invocation
- Always read dependency artifacts before creating a new one - re-read from disk, not from conversation memory (files may have changed since you last saw them)
- Never skip artifacts or create out of order
- If context is unclear, ask the user before creating
- Verify the artifact exists after creating it: a file for file artifacts, a real `gh issue view` response for `tasks`
- Never create `tasks.md`; create or update the GitHub issue instead
- Use the schema's artifact sequence, don't assume specific artifact names
- **IMPORTANT**: `context` and `rules` are constraints for YOU, not content for the file
  - Do NOT copy `<context>`, `<rules>`, `<project_context>` blocks into the artifact
  - These guide what you write, but should never appear in the output
