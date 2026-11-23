-- Additive initialization for the PA#3 PostgreSQL store.
-- Safe to run more than once. Creates missing objects only.
-- Does not remove tables, columns, or rows, and does not rewrite existing records.
-- Domain policy (authorization, deadlines, counted selection, lease fencing,
-- budget decisions) stays in Go. This script has no triggers and no functions.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    display_name text NOT NULL,
    role text NOT NULL,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_idx ON users (lower(email));

CREATE TABLE IF NOT EXISTS sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id),
    token_hash bytea NOT NULL,
    csrf_token_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS sessions_token_hash_idx ON sessions (token_hash);

CREATE TABLE IF NOT EXISTS classes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS memberships (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id),
    class_id uuid NOT NULL REFERENCES classes (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    removed_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS memberships_current_user_class_idx
    ON memberships (user_id, class_id)
    WHERE removed_at IS NULL;

CREATE TABLE IF NOT EXISTS labs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    class_id uuid NOT NULL REFERENCES classes (id),
    created_by uuid NOT NULL REFERENCES users (id),
    title text NOT NULL,
    statement text NOT NULL,
    opens_at timestamptz NOT NULL,
    deadline timestamptz NOT NULL,
    cpu_seconds integer NOT NULL,
    memory_mib integer NOT NULL,
    compile_seconds integer NOT NULL DEFAULT 10,
    wall_multiplier integer NOT NULL DEFAULT 3,
    max_processes integer NOT NULL DEFAULT 16,
    max_output_bytes integer NOT NULL DEFAULT 1048576,
    max_source_bytes integer NOT NULL DEFAULT 65536,
    budget_cap_micros bigint NOT NULL DEFAULT 250000,
    current_revision integer NOT NULL DEFAULT 1,
    test_revision integer NOT NULL DEFAULT 1,
    constraint_revision integer NOT NULL DEFAULT 1,
    generation integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS lab_revisions (
    lab_id uuid NOT NULL REFERENCES labs (id),
    revision integer NOT NULL,
    title text NOT NULL,
    statement text NOT NULL,
    opens_at timestamptz NOT NULL,
    deadline timestamptz NOT NULL,
    cpu_seconds integer NOT NULL,
    memory_mib integer NOT NULL,
    compile_seconds integer NOT NULL,
    wall_multiplier integer NOT NULL,
    max_processes integer NOT NULL,
    max_output_bytes integer NOT NULL,
    max_source_bytes integer NOT NULL,
    test_revision integer NOT NULL,
    constraint_revision integer NOT NULL,
    generation integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (lab_id, revision)
);

CREATE TABLE IF NOT EXISTS lab_tests (
    lab_id uuid NOT NULL,
    revision integer NOT NULL,
    test_id text NOT NULL,
    position integer NOT NULL,
    input text NOT NULL,
    expected_output text NOT NULL,
    sample boolean NOT NULL,
    PRIMARY KEY (lab_id, revision, test_id),
    FOREIGN KEY (lab_id, revision) REFERENCES lab_revisions (lab_id, revision)
);

CREATE TABLE IF NOT EXISTS lab_constraints (
    lab_id uuid NOT NULL,
    revision integer NOT NULL,
    constraint_id text NOT NULL,
    position integer NOT NULL,
    body text NOT NULL,
    deduction_hundredths integer NOT NULL,
    PRIMARY KEY (lab_id, revision, constraint_id),
    FOREIGN KEY (lab_id, revision) REFERENCES lab_revisions (lab_id, revision)
);

CREATE TABLE IF NOT EXISTS submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lab_id uuid NOT NULL REFERENCES labs (id),
    author_user_id uuid NOT NULL REFERENCES users (id),
    source text NOT NULL,
    source_sha256 text NOT NULL,
    state text NOT NULL,
    received_at timestamptz NOT NULL,
    server_seq bigint GENERATED ALWAYS AS IDENTITY,
    generation integer NOT NULL,
    test_revision integer NOT NULL,
    constraint_revision integer NOT NULL,
    judge_verdict text,
    infrastructure_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (server_seq)
);

CREATE INDEX IF NOT EXISTS submissions_lab_author_receipt_idx
    ON submissions (lab_id, author_user_id, received_at, server_seq);

CREATE TABLE IF NOT EXISTS counted_selections (
    lab_id uuid NOT NULL REFERENCES labs (id),
    author_user_id uuid NOT NULL REFERENCES users (id),
    submission_id uuid NOT NULL REFERENCES submissions (id),
    generation integer NOT NULL,
    received_at timestamptz NOT NULL,
    server_seq bigint NOT NULL,
    PRIMARY KEY (lab_id, author_user_id)
);

CREATE TABLE IF NOT EXISTS judge_verdicts (
    submission_id uuid NOT NULL REFERENCES submissions (id),
    test_revision integer NOT NULL,
    constraint_revision integer NOT NULL,
    attempt integer NOT NULL,
    aggregate_verdict text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (submission_id, test_revision, constraint_revision)
);

CREATE TABLE IF NOT EXISTS judge_result_tests (
    submission_id uuid NOT NULL,
    test_revision integer NOT NULL,
    constraint_revision integer NOT NULL,
    test_id text NOT NULL,
    verdict text NOT NULL,
    time_ms integer NOT NULL,
    PRIMARY KEY (submission_id, test_revision, constraint_revision, test_id),
    FOREIGN KEY (submission_id, test_revision, constraint_revision)
        REFERENCES judge_verdicts (submission_id, test_revision, constraint_revision)
);

CREATE TABLE IF NOT EXISTS jobs (
    id uuid PRIMARY KEY,
    submission_id uuid NOT NULL REFERENCES submissions (id),
    lab_id uuid NOT NULL REFERENCES labs (id),
    kind text NOT NULL,
    attempt integer NOT NULL,
    generation integer NOT NULL,
    test_revision integer NOT NULL,
    constraint_revision integer NOT NULL,
    source_sha256 text NOT NULL,
    state text NOT NULL,
    lease_expires_at timestamptz,
    worker_id text,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (submission_id, kind, generation, test_revision, constraint_revision)
);

CREATE INDEX IF NOT EXISTS jobs_state_lease_idx ON jobs (state, lease_expires_at);

CREATE TABLE IF NOT EXISTS findings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id uuid NOT NULL REFERENCES submissions (id),
    lab_id uuid NOT NULL REFERENCES labs (id),
    constraint_id text NOT NULL,
    generation integer NOT NULL,
    test_revision integer NOT NULL,
    constraint_revision integer NOT NULL,
    source_sha256 text NOT NULL,
    source_kind text NOT NULL,
    verdict text NOT NULL,
    unsure_cause text,
    deduction_hundredths integer NOT NULL,
    required_review boolean NOT NULL,
    revision integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (submission_id, constraint_id, generation, test_revision, constraint_revision)
);

CREATE TABLE IF NOT EXISTS finding_evidence (
    finding_id uuid NOT NULL REFERENCES findings (id),
    position integer NOT NULL,
    line integer NOT NULL,
    evidence_text text NOT NULL,
    PRIMARY KEY (finding_id, position)
);

CREATE TABLE IF NOT EXISTS decisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    finding_id uuid NOT NULL REFERENCES findings (id),
    revision integer NOT NULL,
    verdict text NOT NULL,
    reason text NOT NULL,
    evidence_line integer,
    evidence_text text,
    actor_user_id uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (finding_id, revision)
);

CREATE TABLE IF NOT EXISTS replays (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id uuid NOT NULL REFERENCES users (id),
    operation text NOT NULL,
    resource text NOT NULL,
    idempotency_key text NOT NULL,
    request_fingerprint text NOT NULL,
    status_code integer NOT NULL,
    response_body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (actor_user_id, operation, resource, idempotency_key)
);

CREATE TABLE IF NOT EXISTS budget_reservations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lab_id uuid NOT NULL REFERENCES labs (id),
    submission_id uuid REFERENCES submissions (id),
    reserved_micros bigint NOT NULL,
    settled_micros bigint,
    state text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS budget_reservations_lab_state_idx
    ON budget_reservations (lab_id, state);

CREATE TABLE IF NOT EXISTS admission_leases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id),
    lab_id uuid NOT NULL REFERENCES labs (id),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth_rate_limits (
    bucket text NOT NULL,
    subject text NOT NULL,
    window_started_at timestamptz NOT NULL,
    attempts integer NOT NULL,
    PRIMARY KEY (bucket, subject, window_started_at)
);

INSERT INTO schema_migrations (version)
VALUES ('001_init')
ON CONFLICT (version) DO NOTHING;
