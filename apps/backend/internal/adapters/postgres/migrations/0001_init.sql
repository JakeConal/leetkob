-- PA#3 durable store, version 1.
--
-- Additive only: every statement is CREATE ... IF NOT EXISTS and no
-- destructive ALTER or DROP is permitted here. Applied by the migration
-- runner in internal/adapters/postgres and recorded in schema_migrations.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE,
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 100),
    role text NOT NULL CHECK (role IN ('admin', 'ta', 'student')),
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id),
    token_hash bytea NOT NULL UNIQUE,
    csrf_token_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);

CREATE TABLE IF NOT EXISTS classes (
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS memberships (
    class_id uuid NOT NULL REFERENCES classes (id),
    user_id uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (class_id, user_id)
);

CREATE TABLE IF NOT EXISTS labs (
    id uuid PRIMARY KEY,
    class_id uuid NOT NULL REFERENCES classes (id),
    title text NOT NULL,
    statement text NOT NULL,
    opens_at timestamptz NOT NULL,
    deadline timestamptz NOT NULL,
    revision integer NOT NULL CHECK (revision >= 1),
    cpu_seconds integer NOT NULL CHECK (cpu_seconds BETWEEN 1 AND 5),
    memory_mib integer NOT NULL CHECK (memory_mib BETWEEN 64 AND 512),
    model_notice text NOT NULL,
    created_by uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (opens_at < deadline)
);

CREATE TABLE IF NOT EXISTS lab_constraints (
    lab_id uuid NOT NULL REFERENCES labs (id) ON DELETE CASCADE,
    constraint_id text NOT NULL,
    text text NOT NULL,
    deduction_cents integer NOT NULL CHECK (deduction_cents BETWEEN 0 AND 1000),
    revision integer NOT NULL,
    PRIMARY KEY (lab_id, constraint_id)
);

CREATE TABLE IF NOT EXISTS lab_tests (
    lab_id uuid NOT NULL REFERENCES labs (id) ON DELETE CASCADE,
    test_id text NOT NULL,
    input text NOT NULL,
    expected_output text NOT NULL,
    sample boolean NOT NULL,
    position integer NOT NULL,
    PRIMARY KEY (lab_id, test_id)
);

CREATE TABLE IF NOT EXISTS submissions (
    id uuid PRIMARY KEY,
    lab_id uuid NOT NULL REFERENCES labs (id),
    student_id uuid NOT NULL REFERENCES users (id),
    source text NOT NULL,
    source_sha256 bytea NOT NULL,
    state text NOT NULL CHECK (state IN ('queued', 'judging', 'judged')),
    received_at timestamptz NOT NULL,
    receipt_seq bigint NOT NULL,
    generation integer NOT NULL CHECK (generation >= 1),
    counted boolean NOT NULL DEFAULT false,
    verdict text CHECK (verdict IN ('AC', 'WA', 'TLE', 'CE', 'RE', 'MLE')),
    judged_at timestamptz
);

CREATE INDEX IF NOT EXISTS submissions_lab_student_idx ON submissions (lab_id, student_id);

-- At most one counted submission per student and lab; later receipts
-- replace earlier ones by clearing the flag in the same transaction.
CREATE UNIQUE INDEX IF NOT EXISTS submissions_one_counted_idx ON submissions (lab_id, student_id)
WHERE
    counted;

CREATE TABLE IF NOT EXISTS idempotency_keys (
    actor_id uuid NOT NULL,
    operation text NOT NULL,
    resource_id text NOT NULL,
    idempotency_key text NOT NULL,
    fingerprint text NOT NULL,
    status integer NOT NULL,
    content_type text NOT NULL,
    body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, operation, resource_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS jobs (
    id uuid PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('judge', 'check')),
    submission_id uuid NOT NULL REFERENCES submissions (id),
    lab_id uuid NOT NULL REFERENCES labs (id),
    generation integer NOT NULL,
    test_revision integer NOT NULL,
    constraint_revision integer NOT NULL,
    source_sha256 bytea NOT NULL,
    attempt integer NOT NULL DEFAULT 0,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'leased', 'done', 'failed')),
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, submission_id, generation)
);

CREATE TABLE IF NOT EXISTS findings (
    id uuid PRIMARY KEY,
    submission_id uuid NOT NULL REFERENCES submissions (id),
    lab_id uuid NOT NULL REFERENCES labs (id),
    constraint_id text NOT NULL,
    constraint_revision integer NOT NULL,
    source_kind text NOT NULL CHECK (source_kind IN ('hard-rule', 'model')),
    checker_verdict text NOT NULL CHECK (checker_verdict IN ('pass', 'violation', 'unsure')),
    unsure_cause text,
    evidence jsonb NOT NULL DEFAULT '[]'::jsonb,
    deduction_cents integer NOT NULL CHECK (deduction_cents BETWEEN 0 AND 1000),
    required_review boolean NOT NULL,
    revision integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (submission_id, constraint_id, constraint_revision)
);

CREATE TABLE IF NOT EXISTS decisions (
    id uuid PRIMARY KEY,
    finding_id uuid NOT NULL REFERENCES findings (id),
    verdict text NOT NULL CHECK (verdict IN ('pass', 'violation')),
    reason text NOT NULL,
    evidence jsonb,
    finding_revision integer NOT NULL,
    decided_by uuid NOT NULL REFERENCES users (id),
    decided_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (finding_id, finding_revision)
);

CREATE TABLE IF NOT EXISTS budget_reservations (
    id uuid PRIMARY KEY,
    lab_id uuid NOT NULL REFERENCES labs (id),
    reserved_cents integer NOT NULL CHECK (reserved_cents >= 0),
    state text NOT NULL CHECK (state IN ('reserved', 'settled', 'released')),
    settled_cents integer,
    provider_request_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
