"use client";

import Link from "next/link";
import { useRef, useState, useSyncExternalStore, type FormEvent } from "react";

import {
  createIdempotencyKey,
  summarizeError,
  type ApiErrorSummary,
} from "../../../../../lib/api-client";
import type { Lab } from "../../../../../lib/api-types";
import { LAB_BOUNDS, validateLabDraft, type LabDraftErrors } from "../../../../../lib/lab-form";
import { formatTimestamp } from "../../../../../lib/format";
import { getSession, subscribeProtectedState } from "../../../../../lib/protected-state";
import { createLab } from "../../../../../lib/session-client";

const FIELD_ORDER = [
  "title",
  "statement",
  "opensAt",
  "deadline",
  "constraints",
  "tests",
  "cpuSeconds",
  "memoryMiB",
] as const;

const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const EMPTY_DRAFT = {
  title: "",
  statement: "",
  opensAt: "",
  deadline: "",
  constraintsText: "",
  testsText: "",
  cpuSeconds: "1",
  memoryMiB: "256",
} as const;

const CONSTRAINT_PLACEHOLDER = "C1 | Do not call `std::sort`. | 3\nC2 | Implement merge sort. | 4";
const TEST_PLACEHOLDER = `[
  { "id": "01", "input": "3\\n3 1 2\\n", "expectedOutput": "1 2 3\\n", "sample": true }
]`;

function firstErrorField(errors: LabDraftErrors): string | null {
  for (const field of FIELD_ORDER) {
    if (errors[field] !== undefined) return field;
  }
  return null;
}

function ErrorText({ errors, field }: Readonly<{ errors: LabDraftErrors; field: string }>) {
  const message = errors[field];
  if (message === undefined) return null;
  return (
    <p className="field-error" id={`${field}-error`}>
      {message}
    </p>
  );
}

export function LabAuthoringForm({ classId }: Readonly<{ classId: string }>) {
  const session = useSyncExternalStore(subscribeProtectedState, getSession, () => null);
  const [draft, setDraft] = useState({ ...EMPTY_DRAFT });
  const [errors, setErrors] = useState<LabDraftErrors>({});
  const [problem, setProblem] = useState<ApiErrorSummary | null>(null);
  const [pending, setPending] = useState(false);
  const [created, setCreated] = useState<Lab | null>(null);
  const idempotencyKey = useRef<string | null>(null);

  const validClassId = UUID_PATTERN.test(classId);

  function update(field: keyof typeof EMPTY_DRAFT, value: string): void {
    setDraft((current) => ({ ...current, [field]: value }));
    setErrors((current) => {
      if (current[field] === undefined) return current;
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setProblem(null);
    setCreated(null);

    const result = validateLabDraft(draft);
    if (!result.ok) {
      setErrors(result.errors);
      const field = firstErrorField(result.errors);
      if (field !== null) document.getElementById(field)?.focus();
      return;
    }

    setErrors({});
    setPending(true);
    const key = idempotencyKey.current ?? createIdempotencyKey();
    idempotencyKey.current = key;
    try {
      const lab = await createLab(classId, result.value, { idempotencyKey: key });
      idempotencyKey.current = null;
      setCreated(lab);
    } catch (error) {
      setProblem(summarizeError(error));
    } finally {
      setPending(false);
    }
  }

  if (session === null) {
    return (
      <section className="panel" aria-labelledby="signed-out-heading">
        <div className="panel-heading">
          <h2 id="signed-out-heading">Sign in required</h2>
          <p>
            The CSRF proof is kept in memory only, so authoring needs a fresh sign-in on this page.
          </p>
        </div>
        <div className="actions">
          <Link className="primary-action" href="/sign-in">
            Sign in
          </Link>
        </div>
      </section>
    );
  }

  if (!validClassId) {
    return (
      <section className="panel" aria-labelledby="invalid-class-heading">
        <div className="panel-heading">
          <h2 id="invalid-class-heading">Invalid class identifier</h2>
          <p role="alert">A class ID must be a UUID version 1-5.</p>
        </div>
        <div className="actions">
          <Link className="inline-action" href="/classes">
            Back to classes
          </Link>
        </div>
      </section>
    );
  }

  if (session.user.role !== "ta") {
    return (
      <section className="panel" aria-labelledby="role-heading">
        <div className="panel-heading">
          <h2 id="role-heading">TA access required</h2>
          <p>
            Signed in as {session.user.displayName} ({session.user.role}). Only an assigned TA can
            create a lab; the API enforces this even if the form is reached directly.
          </p>
        </div>
        <div className="actions">
          <Link className="inline-action" href="/classes">
            Back to classes
          </Link>
        </div>
      </section>
    );
  }

  if (created !== null) {
    return (
      <section className="panel" aria-labelledby="created-heading">
        <div className="panel-heading">
          <p className="panel-label">Created</p>
          <h2 id="created-heading">{created.title}</h2>
          <p role="status">
            Revision {created.revision} stored atomically with its tests. No partial lab is visible
            on failure.
          </p>
        </div>
        <dl className="lab-facts">
          <div>
            <dt>Class</dt>
            <dd>{created.classId}</dd>
          </div>
          <div>
            <dt>Opens</dt>
            <dd>{formatTimestamp(created.opensAt)}</dd>
          </div>
          <div>
            <dt>Deadline</dt>
            <dd>{formatTimestamp(created.deadline)}</dd>
          </div>
          <div>
            <dt>Revision</dt>
            <dd>{created.revision}</dd>
          </div>
        </dl>
        <p className="model-notice-text">{created.modelNotice}</p>
        <div className="actions">
          <Link className="primary-action" href={`/labs/${created.id}`}>
            Open lab
          </Link>
          <button className="secondary-action" type="button" onClick={() => setCreated(null)}>
            Author another lab
          </button>
        </div>
      </section>
    );
  }

  return (
    <section className="panel" aria-labelledby="form-heading">
      <div className="panel-heading">
        <p className="panel-label">Authoring</p>
        <h2 id="form-heading">Lab definition</h2>
        <p>
          Limits are C++17 at -O2. No filesystem paths or compiler command are accepted. Class ID:{" "}
          <code>{classId}</code>
        </p>
      </div>

      <form className="form-stack" onSubmit={(event) => void handleSubmit(event)} noValidate>
        <div className="field">
          <label htmlFor="title">Title</label>
          <input
            id="title"
            name="title"
            type="text"
            required
            maxLength={LAB_BOUNDS.title.maxLength}
            value={draft.title}
            onChange={(event) => update("title", event.target.value)}
            aria-invalid={errors["title"] !== undefined}
            aria-describedby={
              errors["title"] === undefined ? "title-hint" : "title-hint title-error"
            }
          />
          <p className="field-hint" id="title-hint">
            Up to {LAB_BOUNDS.title.maxLength} characters.
          </p>
          <ErrorText errors={errors} field="title" />
        </div>

        <div className="field">
          <label htmlFor="statement">Statement</label>
          <textarea
            id="statement"
            name="statement"
            rows={8}
            required
            value={draft.statement}
            onChange={(event) => update("statement", event.target.value)}
            aria-invalid={errors["statement"] !== undefined}
            aria-describedby={
              errors["statement"] === undefined
                ? "statement-hint"
                : "statement-hint statement-error"
            }
          />
          <p className="field-hint" id="statement-hint">
            Up to {LAB_BOUNDS.statement.maxLength} characters. Markdown is stored as plain text.
          </p>
          <ErrorText errors={errors} field="statement" />
        </div>

        <div className="field-row">
          <div className="field">
            <label htmlFor="opensAt">Opens at</label>
            <input
              id="opensAt"
              name="opensAt"
              type="datetime-local"
              required
              value={draft.opensAt}
              onChange={(event) => update("opensAt", event.target.value)}
              aria-invalid={errors["opensAt"] !== undefined}
              aria-describedby={errors["opensAt"] === undefined ? "opensAt-hint" : "opensAt-error"}
            />
            <p className="field-hint" id="opensAt-hint">
              Local time; converted to UTC for the API.
            </p>
            <ErrorText errors={errors} field="opensAt" />
          </div>

          <div className="field">
            <label htmlFor="deadline">Deadline</label>
            <input
              id="deadline"
              name="deadline"
              type="datetime-local"
              required
              value={draft.deadline}
              onChange={(event) => update("deadline", event.target.value)}
              aria-invalid={errors["deadline"] !== undefined}
              aria-describedby={
                errors["deadline"] === undefined ? "deadline-hint" : "deadline-error"
              }
            />
            <p className="field-hint" id="deadline-hint">
              Must be strictly after the opening time.
            </p>
            <ErrorText errors={errors} field="deadline" />
          </div>
        </div>

        <div className="field">
          <label htmlFor="constraints">Constraints</label>
          <textarea
            id="constraints"
            name="constraints"
            rows={5}
            required
            spellCheck={false}
            placeholder={CONSTRAINT_PLACEHOLDER}
            value={draft.constraintsText}
            onChange={(event) => update("constraintsText", event.target.value)}
            aria-invalid={errors["constraints"] !== undefined}
            aria-describedby={
              errors["constraints"] === undefined
                ? "constraints-hint"
                : "constraints-hint constraints-error"
            }
          />
          <p className="field-hint" id="constraints-hint">
            One per line as <code>ID | text | deduction</code>. 1-{LAB_BOUNDS.constraints.maxItems}{" "}
            unique IDs; deductions 0-10 with at most two decimals. Ban-list texts start with “Do not
            use” or “Do not call”; backticks name the banned symbol.
          </p>
          <ErrorText errors={errors} field="constraints" />
        </div>

        <div className="field">
          <label htmlFor="tests">Tests (JSON array)</label>
          <textarea
            id="tests"
            name="tests"
            rows={8}
            required
            spellCheck={false}
            placeholder={TEST_PLACEHOLDER}
            value={draft.testsText}
            onChange={(event) => update("testsText", event.target.value)}
            aria-invalid={errors["tests"] !== undefined}
            aria-describedby={
              errors["tests"] === undefined ? "tests-hint" : "tests-hint tests-error"
            }
          />
          <p className="field-hint" id="tests-hint">
            Array of <code>{`{"id","input","expectedOutput","sample"}`}</code>, 1-
            {LAB_BOUNDS.tests.maxItems} tests with unique IDs and at most 10 MiB of combined
            input/output. Only <code>sample: true</code> tests are ever visible to students.
          </p>
          <ErrorText errors={errors} field="tests" />
        </div>

        <fieldset className="field-row fieldset">
          <legend>Limits</legend>
          <div className="field">
            <label htmlFor="cpuSeconds">CPU seconds</label>
            <input
              id="cpuSeconds"
              name="cpuSeconds"
              type="number"
              min={LAB_BOUNDS.limits.cpuSeconds.min}
              max={LAB_BOUNDS.limits.cpuSeconds.max}
              step={1}
              required
              value={draft.cpuSeconds}
              onChange={(event) => update("cpuSeconds", event.target.value)}
              aria-invalid={errors["cpuSeconds"] !== undefined}
              aria-describedby={errors["cpuSeconds"] === undefined ? undefined : "cpuSeconds-error"}
            />
            <p className="field-hint">
              1-{LAB_BOUNDS.limits.cpuSeconds.max} seconds; wall time is three times CPU.
            </p>
            <ErrorText errors={errors} field="cpuSeconds" />
          </div>

          <div className="field">
            <label htmlFor="memoryMiB">Memory (MiB)</label>
            <input
              id="memoryMiB"
              name="memoryMiB"
              type="number"
              min={LAB_BOUNDS.limits.memoryMiB.min}
              max={LAB_BOUNDS.limits.memoryMiB.max}
              step={1}
              required
              value={draft.memoryMiB}
              onChange={(event) => update("memoryMiB", event.target.value)}
              aria-invalid={errors["memoryMiB"] !== undefined}
              aria-describedby={errors["memoryMiB"] === undefined ? undefined : "memoryMiB-error"}
            />
            <p className="field-hint">
              {LAB_BOUNDS.limits.memoryMiB.min}-{LAB_BOUNDS.limits.memoryMiB.max} MiB.
            </p>
            <ErrorText errors={errors} field="memoryMiB" />
          </div>
        </fieldset>

        {problem === null ? null : (
          <div className="problem-alert" role="alert">
            <p className="problem-alert-title">{problem.title}</p>
            <p>{problem.detail}</p>
            <p className="problem-meta">
              {problem.status === null ? null : <span>HTTP {problem.status}</span>}
              {problem.type === null ? null : <span>{problem.type}</span>}
            </p>
          </div>
        )}

        <div className="actions">
          <button className="primary-action" type="submit" disabled={pending}>
            {pending ? "Creating lab…" : "Create lab"}
          </button>
          <Link className="inline-action" href="/classes">
            Back to classes
          </Link>
        </div>
      </form>
    </section>
  );
}
