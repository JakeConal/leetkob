-- Review sampling support, version 5.
-- Additive only: a defaulted column that records the persisted pass sample.
--
-- Findings already record required_review. The queue additionally marks the
-- stable random ceil(10%) pass sample with sampled = true. Membership is
-- recomputed only from immutable finding IDs and persisted here, so queue
-- reads never redraw an unchanged sample.

ALTER TABLE findings
ADD COLUMN IF NOT EXISTS sampled boolean NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS findings_lab_checker_verdict_idx
ON findings (lab_id, checker_verdict);
