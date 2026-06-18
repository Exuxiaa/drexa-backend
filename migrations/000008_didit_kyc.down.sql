-- 000008_didit_kyc.down.sql
-- Reverse 000008: drop event idempotency table and Didit session columns.

DROP TABLE IF EXISTS didit_processed_events;

DROP INDEX IF EXISTS idx_kyc_didit_session_id;

ALTER TABLE kyc_submissions
    DROP COLUMN IF EXISTS didit_session_id,
    DROP COLUMN IF EXISTS didit_session_url,
    DROP COLUMN IF EXISTS didit_status;
