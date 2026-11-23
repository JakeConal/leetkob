"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { FormEvent } from "react";

import { ApiProblem, createLab, getCurrentUser } from "../../lib/live/api";
import { LiveError } from "./live-status";
import { useLiveSession } from "./live-shell";

interface ConstraintDraft {
  readonly id: string;
  readonly text: string;
  readonly deduction: string;
}

interface TestDraft {
  readonly id: string;
  readonly input: string;
  readonly expectedOutput: string;
}

const emptyConstraint = (): ConstraintDraft => ({ id: "", text: "", deduction: "" });
const emptyTest = (): TestDraft => ({ id: "", input: "", expectedOutput: "" });

function parseDeduction(raw: string): number | null {
  const trimmed = raw.trim();
  if (!/^\d{1,2}(\.\d{1,2})?$/.test(trimmed)) return null;
  const value = Number(trimmed);
  if (value < 0 || value > 10) return null;
  return value;
}

function toIso(local: string): string | null {
  if (local.trim().length === 0) return null;
  const time = new Date(local);
  if (Number.isNaN(time.getTime())) return null;
  return time.toISOString();
}

function collectTests(
  drafts: readonly TestDraft[],
  sample: boolean,
): { id: string; input: string; expectedOutput: string; sample: boolean }[] | string {
  const tests: { id: string; input: string; expectedOutput: string; sample: boolean }[] = [];
  for (const draft of drafts) {
    const blank = draft.id === "" && draft.input === "" && draft.expectedOutput === "";
    if (blank) continue;
    if (draft.id === "" || draft.input === "" || draft.expectedOutput === "") {
      return "Finish every test row or clear it.";
    }
    tests.push({
      id: draft.id.trim(),
      input: draft.input,
      expectedOutput: draft.expectedOutput,
      sample,
    });
  }
  return tests;
}

export function CreateLabForm({ classId }: { readonly classId: string }) {
  const session = useLiveSession();
  const router = useRouter();
  const me = useQuery({
    queryKey: ["live", "me"],
    queryFn: getCurrentUser,
    enabled: session.ready && !session.closing,
    retry: false,
    gcTime: 0,
    staleTime: 0,
  });
  const signedOut = me.error instanceof ApiProblem && me.error.status === 401;
  useEffect(() => {
    if (signedOut) router.replace("/live");
  }, [router, signedOut]);
  const [title, setTitle] = useState("");
  const [statement, setStatement] = useState("");
  const [opensAt, setOpensAt] = useState("");
  const [deadline, setDeadline] = useState("");
  const [cpuSeconds, setCpuSeconds] = useState("1");
  const [memoryMiB, setMemoryMiB] = useState("256");
  const [constraints, setConstraints] = useState<[ConstraintDraft, ConstraintDraft]>([
    emptyConstraint(),
    emptyConstraint(),
  ]);
  const [sampleTests, setSampleTests] = useState<TestDraft[]>([emptyTest()]);
  const [hiddenTests, setHiddenTests] = useState<TestDraft[]>([emptyTest()]);
  const [formError, setFormError] = useState<unknown>(null);
  const [created, setCreated] = useState<{ id: string; title: string } | null>(null);
  const [pending, setPending] = useState(false);

  function updateConstraint(index: 0 | 1, patch: Partial<ConstraintDraft>) {
    setConstraints((current) => {
      const next: [ConstraintDraft, ConstraintDraft] = [...current];
      const row = next[index];
      next[index] = { ...row, ...patch };
      return next;
    });
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreated(null);
    setFormError(null);
    if (session.csrfToken === null) {
      setFormError(new Error("Sign in again before creating a lab."));
      return;
    }
    const opens = toIso(opensAt);
    const closes = toIso(deadline);
    if (title.trim() === "" || statement.trim() === "") {
      setFormError(new Error("Enter a title and a statement."));
      return;
    }
    if (opens === null || closes === null) {
      setFormError(new Error("Enter an opening time and a deadline."));
      return;
    }
    if (Date.parse(closes) <= Date.parse(opens)) {
      setFormError(new Error("The deadline must be after the opening time."));
      return;
    }
    const parsedConstraints = [];
    for (const row of constraints) {
      const deduction = parseDeduction(row.deduction);
      if (row.id.trim() === "" || row.text.trim() === "" || deduction === null) {
        setFormError(new Error("Each constraint needs an id, text, and a deduction from 0 to 10."));
        return;
      }
      parsedConstraints.push({ id: row.id.trim(), text: row.text.trim(), deduction });
    }
    if (parsedConstraints[0]?.id === parsedConstraints[1]?.id) {
      setFormError(new Error("Constraint ids must be different."));
      return;
    }
    const cpu = Number(cpuSeconds);
    const memory = Number(memoryMiB);
    if (!Number.isInteger(cpu) || cpu < 1 || cpu > 5) {
      setFormError(new Error("CPU must be a whole number of seconds from 1 to 5."));
      return;
    }
    if (!Number.isInteger(memory) || memory < 64 || memory > 512) {
      setFormError(new Error("Memory must be a whole number of MiB from 64 to 512."));
      return;
    }
    const sample = collectTests(sampleTests, true);
    const hidden = collectTests(hiddenTests, false);
    if (typeof sample === "string") {
      setFormError(new Error(sample));
      return;
    }
    if (typeof hidden === "string") {
      setFormError(new Error(hidden));
      return;
    }
    const tests = [...sample, ...hidden];
    if (tests.length === 0) {
      setFormError(new Error("Add at least one sample or hidden test."));
      return;
    }
    setPending(true);
    try {
      const lab = await createLab(
        classId,
        {
          title: title.trim(),
          statement,
          opensAt: opens,
          deadline: closes,
          constraints: parsedConstraints,
          tests,
          limits: { cpuSeconds: cpu, memoryMiB: memory },
        },
        session.csrfToken,
        crypto.randomUUID(),
      );
      setCreated({ id: lab.id, title: lab.title });
    } catch (error) {
      setCreated(null);
      setFormError(error);
    } finally {
      setPending(false);
    }
  }

  if (!session.ready || session.closing || me.isPending) {
    return <p className="live-muted">Loading…</p>;
  }
  if (signedOut) return <p className="live-muted">Sign in required.</p>;
  if (!me.isSuccess) return <LiveError error={me.error} />;
  if (me.data.user.role !== "ta") {
    return (
      <section className="live-card">
        <h1>Create lab</h1>
        <p>Only an assigned teaching assistant can create a lab.</p>
      </section>
    );
  }

  return (
    <form className="live-card live-form" onSubmit={(event) => void onSubmit(event)}>
      <h1>Create lab</h1>
      <p className="live-muted">
        The lab is created only when the API accepts it. An error leaves this form in place.
      </p>
      {formError ? <LiveError error={formError} /> : null}
      {created ? (
        <p role="status">
          Created {created.title}. <Link href={`/live/labs/${created.id}`}>Open lab</Link>
        </p>
      ) : null}
      <label className="live-field" htmlFor="lab-title">
        Title
        <input
          id="lab-title"
          value={title}
          required
          onChange={(event) => setTitle(event.target.value)}
        />
      </label>
      <label className="live-field" htmlFor="lab-statement">
        Statement
        <textarea
          id="lab-statement"
          value={statement}
          required
          rows={8}
          onChange={(event) => setStatement(event.target.value)}
        />
      </label>
      <div className="live-panels">
        <label className="live-field" htmlFor="lab-opens">
          Opens
          <input
            id="lab-opens"
            type="datetime-local"
            required
            value={opensAt}
            onChange={(event) => setOpensAt(event.target.value)}
          />
        </label>
        <label className="live-field" htmlFor="lab-deadline">
          Deadline
          <input
            id="lab-deadline"
            type="datetime-local"
            required
            value={deadline}
            onChange={(event) => setDeadline(event.target.value)}
          />
        </label>
      </div>
      <fieldset className="live-choice">
        <legend>Limits</legend>
        <label className="live-field" htmlFor="lab-cpu">
          CPU seconds (1–5)
          <input
            id="lab-cpu"
            inputMode="numeric"
            value={cpuSeconds}
            onChange={(event) => setCpuSeconds(event.target.value)}
          />
        </label>
        <label className="live-field" htmlFor="lab-memory">
          Memory MiB (64–512)
          <input
            id="lab-memory"
            inputMode="numeric"
            value={memoryMiB}
            onChange={(event) => setMemoryMiB(event.target.value)}
          />
        </label>
      </fieldset>
      {constraints.map((row, index) => (
        <fieldset className="live-choice" key={index}>
          <legend>Constraint {index + 1}</legend>
          <label className="live-field" htmlFor={`constraint-id-${index}`}>
            Id
            <input
              id={`constraint-id-${index}`}
              value={row.id}
              onChange={(event) => updateConstraint(index as 0 | 1, { id: event.target.value })}
            />
          </label>
          <label className="live-field" htmlFor={`constraint-text-${index}`}>
            Text
            <textarea
              id={`constraint-text-${index}`}
              rows={3}
              value={row.text}
              onChange={(event) => updateConstraint(index as 0 | 1, { text: event.target.value })}
            />
          </label>
          <label className="live-field" htmlFor={`constraint-deduction-${index}`}>
            Deduction
            <input
              id={`constraint-deduction-${index}`}
              inputMode="decimal"
              value={row.deduction}
              onChange={(event) =>
                updateConstraint(index as 0 | 1, { deduction: event.target.value })
              }
            />
          </label>
        </fieldset>
      ))}
      <TestEditor label="Sample tests" tests={sampleTests} onChange={setSampleTests} />
      <TestEditor label="Hidden tests" tests={hiddenTests} onChange={setHiddenTests} />
      <button
        className="live-button"
        type="submit"
        disabled={pending || session.csrfToken === null}
      >
        {pending ? "Creating…" : "Create lab"}
      </button>
    </form>
  );
}

function TestEditor({
  label,
  tests,
  onChange,
}: {
  readonly label: string;
  readonly tests: readonly TestDraft[];
  readonly onChange: (tests: TestDraft[]) => void;
}) {
  const prefix = label.toLowerCase().replace(/\s+/g, "-");
  return (
    <fieldset className="live-choice">
      <legend>{label}</legend>
      {tests.map((test, index) => (
        <div className="live-stack" key={`${prefix}-${index}`}>
          <label className="live-field" htmlFor={`${prefix}-id-${index}`}>
            Test id
            <input
              id={`${prefix}-id-${index}`}
              value={test.id}
              onChange={(event) => {
                const next = tests.map((item, itemIndex) =>
                  itemIndex === index ? { ...item, id: event.target.value } : item,
                );
                onChange(next);
              }}
            />
          </label>
          <label className="live-field" htmlFor={`${prefix}-input-${index}`}>
            Input
            <textarea
              id={`${prefix}-input-${index}`}
              rows={3}
              spellCheck={false}
              value={test.input}
              onChange={(event) => {
                const next = tests.map((item, itemIndex) =>
                  itemIndex === index ? { ...item, input: event.target.value } : item,
                );
                onChange(next);
              }}
            />
          </label>
          <label className="live-field" htmlFor={`${prefix}-output-${index}`}>
            Expected output
            <textarea
              id={`${prefix}-output-${index}`}
              rows={3}
              spellCheck={false}
              value={test.expectedOutput}
              onChange={(event) => {
                const next = tests.map((item, itemIndex) =>
                  itemIndex === index ? { ...item, expectedOutput: event.target.value } : item,
                );
                onChange(next);
              }}
            />
          </label>
        </div>
      ))}
      <button
        className="live-button live-button-secondary"
        type="button"
        onClick={() => onChange([...tests, emptyTest()])}
      >
        Add {label.toLowerCase().replace(/s$/, "")}
      </button>
    </fieldset>
  );
}
