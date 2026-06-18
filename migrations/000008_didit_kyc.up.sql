-- 000008_didit_kyc.up.sql
-- Extend kyc_submissions with Didit session fields and add event idempotency table.

-- Didit session tracking columns on the existing submissions table.
ALTER TABLE kyc_submissions
    ADD COLUMN IF NOT EXISTS didit_session_id  TEXT,
    ADD COLUMN IF NOT EXISTS didit_session_url TEXT,
    ADD COLUMN IF NOT EXISTS didit_status      TEXT;

-- Fast lookup when a webhook arrives with a session_id.
CREATE INDEX IF NOT EXISTS idx_kyc_didit_session_id ON kyc_submissions(didit_session_id);

-- One row per delivered webhook event_id — prevents double-processing on Didit retries.
CREATE TABLE IF NOT EXISTS didit_processed_events (
    event_id     TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
