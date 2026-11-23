export type JudgeVerdict = "AC" | "WA" | "TLE" | "CE" | "RE" | "MLE";
export type SubmissionState = "queued" | "judging" | "judged";

const JUDGE_VERDICTS = new Set<JudgeVerdict>(["AC", "WA", "TLE", "CE", "RE", "MLE"]);
const MODEL_VERDICTS = new Set(["pass", "violation", "unsure"]);
const SUBMISSION_STATES = new Set<SubmissionState>(["queued", "judging", "judged"]);
const FINDING_KEYS = new Set([
  "checkerVerdict",
  "evidence",
  "deduction",
  "source",
  "unsureCause",
  "decision",
  "requiredReview",
  "findings",
  "finding",
  "modelVerdict",
  "provisionalDeduction",
  "constraintId",
  "constraintRevision",
]);

export interface JudgeTestView {
  readonly id: string;
  readonly verdict: JudgeVerdict;
  readonly timeMs: number;
}

export interface JudgeView {
  readonly verdict: JudgeVerdict;
  readonly tests: readonly JudgeTestView[];
}

export interface StudentStatus {
  readonly id: string;
  readonly labId: string;
  readonly state: SubmissionState;
  readonly receivedAt: string;
  readonly judge?: JudgeView;
}

export interface SampleTestView {
  readonly id: string;
  readonly input: string;
  readonly expectedOutput: string;
}

export interface ConstraintView {
  readonly id: string;
  readonly text: string;
  readonly deduction: number;
}

export interface StudentLabView {
  readonly id: string;
  readonly classId: string;
  readonly title: string;
  readonly statement: string;
  readonly opensAt: string;
  readonly deadline: string;
  readonly revision: number;
  readonly modelNotice: string;
  readonly constraints: readonly ConstraintView[];
  readonly limits: { readonly cpuSeconds: number; readonly memoryMiB: number };
  readonly sampleTests: readonly SampleTestView[];
}

export interface PublicationGate {
  readonly disabled: boolean;
  readonly title: "Publication blocked";
  readonly explanation: string;
  readonly deadlinePassed: boolean | null;
  readonly reviewPending: number;
  readonly processingPending: boolean;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function expectString(value: unknown, label: string): string {
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`student payload is missing ${label}`);
  }
  return value;
}

function ownKeys(value: unknown): string[] {
  if (Array.isArray(value)) return value.flatMap((item) => ownKeys(item));
  if (!isRecord(value)) return [];
  return Object.keys(value).flatMap((key) => [key, ...ownKeys(value[key])]);
}

export function projectStudentStatus(payload: unknown): StudentStatus {
  if (!isRecord(payload)) throw new Error("student payload is not an object");
  const state = payload["state"];
  if (typeof state !== "string" || !SUBMISSION_STATES.has(state as SubmissionState)) {
    throw new Error("student payload has no judge state");
  }
  const status: StudentStatus = {
    id: expectString(payload["id"], "id"),
    labId: expectString(payload["labId"], "labId"),
    state: state as SubmissionState,
    receivedAt: expectString(payload["receivedAt"], "receivedAt"),
  };
  const judgeRaw = payload["judge"];
  if (judgeRaw === undefined || judgeRaw === null) {
    if (ownKeys(status).some((key) => FINDING_KEYS.has(key))) {
      throw new Error("student payload includes finding fields");
    }
    return status;
  }
  if (!isRecord(judgeRaw)) throw new Error("student payload judge is not an object");
  const verdict = judgeRaw["verdict"];
  if (typeof verdict === "string" && MODEL_VERDICTS.has(verdict)) {
    throw new Error("student payload includes a model verdict");
  }
  if (typeof verdict !== "string" || !JUDGE_VERDICTS.has(verdict as JudgeVerdict)) {
    throw new Error("student payload judge verdict is not a judge state");
  }
  if (!Array.isArray(judgeRaw["tests"])) throw new Error("student payload judge tests are missing");
  const tests: JudgeTestView[] = [];
  for (const item of judgeRaw["tests"]) {
    if (!isRecord(item)) throw new Error("student payload judge test is not an object");
    const testVerdict = item["verdict"];
    if (typeof testVerdict === "string" && MODEL_VERDICTS.has(testVerdict)) {
      throw new Error("student payload includes a model verdict");
    }
    if (typeof testVerdict !== "string" || !JUDGE_VERDICTS.has(testVerdict as JudgeVerdict)) {
      throw new Error("student payload judge verdict is not a judge state");
    }
    const timeMs = item["timeMs"];
    if (typeof timeMs !== "number" || !Number.isInteger(timeMs) || timeMs < 0) {
      throw new Error("student payload is missing test timing");
    }
    tests.push({
      id: expectString(item["id"], "test id"),
      verdict: testVerdict as JudgeVerdict,
      timeMs,
    });
  }
  const view: StudentStatus = { ...status, judge: { verdict: verdict as JudgeVerdict, tests } };
  if (ownKeys(view).some((key) => FINDING_KEYS.has(key))) {
    throw new Error("student payload includes finding fields");
  }
  return view;
}

function expectLabString(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`lab payload is missing ${key}`);
  }
  return value;
}

export function projectStudentLab(payload: unknown): StudentLabView {
  if (!isRecord(payload)) throw new Error("lab payload is not an object");
  const revision = payload["revision"];
  if (typeof revision !== "number" || !Number.isInteger(revision) || revision < 1) {
    throw new Error("lab payload is missing revision");
  }
  const limits = payload["limits"];
  if (!isRecord(limits)) throw new Error("lab payload is missing limits");
  const cpuSeconds = limits["cpuSeconds"];
  const memoryMiB = limits["memoryMiB"];
  if (typeof cpuSeconds !== "number" || typeof memoryMiB !== "number") {
    throw new Error("lab payload is missing limits");
  }
  if (!Array.isArray(payload["constraints"])) throw new Error("lab payload is missing constraints");
  const constraints: ConstraintView[] = [];
  for (const item of payload["constraints"]) {
    if (!isRecord(item)) throw new Error("lab payload constraint is not an object");
    const deduction = item["deduction"];
    if (typeof deduction !== "number")
      throw new Error("lab payload constraint deduction is missing");
    constraints.push({
      id: expectLabString(item, "id"),
      text: expectLabString(item, "text"),
      deduction,
    });
  }
  if (!Array.isArray(payload["sampleTests"]))
    throw new Error("lab payload is missing sample tests");
  const sampleTests: SampleTestView[] = [];
  for (const item of payload["sampleTests"]) {
    if (!isRecord(item) || item["sample"] !== true) continue;
    sampleTests.push({
      id: expectLabString(item, "id"),
      input: typeof item["input"] === "string" ? item["input"] : "",
      expectedOutput: typeof item["expectedOutput"] === "string" ? item["expectedOutput"] : "",
    });
  }
  return {
    id: expectLabString(payload, "id"),
    classId: expectLabString(payload, "classId"),
    title: expectLabString(payload, "title"),
    statement: expectLabString(payload, "statement"),
    opensAt: expectLabString(payload, "opensAt"),
    deadline: expectLabString(payload, "deadline"),
    revision,
    modelNotice: expectLabString(payload, "modelNotice"),
    constraints,
    limits: { cpuSeconds, memoryMiB },
    sampleTests,
  };
}

export function livePublicationGate(input: {
  readonly reviewPending: number;
  readonly processingPending: boolean;
  readonly deadlinePassed: boolean | null;
}): PublicationGate {
  let explanation: string;
  if (input.reviewPending > 0) {
    const noun = input.reviewPending === 1 ? "review remains" : "reviews remain";
    explanation = `Publication blocked. Every review must be resolved before publishing. ${input.reviewPending} pending ${noun}.`;
  } else if (input.processingPending) {
    explanation =
      "Publication blocked. Processing is incomplete while judging or checking is still running.";
  } else if (input.deadlinePassed === null) {
    explanation = "Publication blocked. The submission deadline has not been loaded.";
  } else if (!input.deadlinePassed) {
    explanation = "Publication blocked. The submission deadline has not passed.";
  } else {
    explanation = "Publication blocked. Production publication is unavailable in this checkpoint.";
  }
  return {
    disabled: true,
    title: "Publication blocked",
    explanation,
    deadlinePassed: input.deadlinePassed,
    reviewPending: input.reviewPending,
    processingPending: input.processingPending,
  };
}
