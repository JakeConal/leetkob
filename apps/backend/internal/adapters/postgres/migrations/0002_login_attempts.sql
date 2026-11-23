-- Login rate limiting, version 2.
-- Additive only: new table, no changes to existing rows.

CREATE TABLE IF NOT EXISTS login_attempts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    attempted_at timestamptz NOT NULL DEFAULT now(),
    client_ip text NOT NULL,
    account_key text NOT NULL
);

CREATE INDEX IF NOT EXISTS login_attempts_ip_idx ON login_attempts (attempted_at, client_ip);

CREATE INDEX IF NOT EXISTS login_attempts_account_idx ON login_attempts (attempted_at, account_key);
