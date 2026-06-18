-- 000001_auth.up.sql
-- Users, refresh tokens, OTP codes

CREATE TABLE IF NOT EXISTS users (
    user_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email            TEXT NOT NULL UNIQUE,
    phone            TEXT NOT NULL UNIQUE,
    password_hash    TEXT NOT NULL,
    trading_pin_hash TEXT NOT NULL DEFAULT '',
    role             TEXT NOT NULL DEFAULT 'user'
                         CHECK (role IN ('user', 'merchant', 'admin')),
    kyc_level        INTEGER NOT NULL DEFAULT 0,
    two_fa_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    two_fa_secret    TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    token_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    user_agent TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    expired_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);

CREATE TABLE IF NOT EXISTS otp_codes (
    otp_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key        TEXT NOT NULL UNIQUE,
    code_hash  TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
