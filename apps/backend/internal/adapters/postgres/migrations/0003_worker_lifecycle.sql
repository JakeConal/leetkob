-- Worker lifecycle support, version 3.
-- Additive only: new table + additive column defaults.

CREATE TABLE IF NOT EXISTS submission_test_results (
    submission_id uuid NOT NULL REFERENCES submissions (id),
    test_id text NOT NULL,
    verdict text NOT NULL CHECK (verdict IN ('AC', 'WA', 'TLE', 'CE', 'RE', 'MLE')),
    time_ms integer NOT NULL CHECK (time_ms >= 0),
    PRIMARY KEY (submission_id, test_id)
);

ALTER TABLE jobs
ADD COLUMN IF NOT EXISTS failure_count integer NOT NULL DEFAULT 0;
