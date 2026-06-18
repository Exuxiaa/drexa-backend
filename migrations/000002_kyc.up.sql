-- 000002_kyc.up.sql
-- KYC submissions

CREATE TABLE IF NOT EXISTS kyc_submissions (
    submission_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    status           TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'approved', 'rejected')),
    full_name        TEXT NOT NULL,
    id_number        TEXT NOT NULL,   -- AES-256 encrypted NIK; never store plaintext
    id_type          TEXT NOT NULL,   -- e.g. 'ktp', 'passport'
    file_url         TEXT NOT NULL,   -- encrypted object storage path
    selfie_url       TEXT NOT NULL,   -- encrypted object storage path
    rejection_reason TEXT,
    submitted_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_by      TEXT NOT NULL DEFAULT '',
    reviewed_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kyc_submissions_user_id  ON kyc_submissions(user_id);
CREATE INDEX IF NOT EXISTS idx_kyc_submissions_status   ON kyc_submissions(status);
