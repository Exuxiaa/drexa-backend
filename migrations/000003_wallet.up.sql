-- 000003_wallet.up.sql
-- Wallets, transactions (audit trail), and fiat deposit/withdrawal requests.
-- Balances are stored as BIGINT in the smallest unit (cents for IDR, satoshi for BTC).

CREATE TABLE IF NOT EXISTS wallets (
    wallet_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    currency    TEXT NOT NULL,
    balance     BIGINT NOT NULL DEFAULT 0,
    locked      BIGINT NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'suspended', 'closed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (user_id, currency)
);

CREATE INDEX IF NOT EXISTS idx_wallets_user_id    ON wallets(user_id);
CREATE INDEX IF NOT EXISTS idx_wallets_currency   ON wallets(currency);
CREATE INDEX IF NOT EXISTS idx_wallets_deleted_at ON wallets(deleted_at);

CREATE TABLE IF NOT EXISTS transactions (
    tx_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id      UUID NOT NULL REFERENCES wallets(wallet_id),
    user_id        UUID NOT NULL REFERENCES users(user_id),
    type           TEXT NOT NULL CHECK (type IN (
                       'deposit', 'withdrawal', 'transfer', 'fee', 'reversal')),
    status         TEXT NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'completed', 'failed', 'reversed')),
    amount         BIGINT NOT NULL,
    balance_before BIGINT NOT NULL DEFAULT 0,
    balance_after  BIGINT NOT NULL DEFAULT 0,
    currency       TEXT NOT NULL,
    ref_id         TEXT NOT NULL DEFAULT '',
    description    TEXT NOT NULL DEFAULT '',
    metadata       TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_transactions_wallet_id ON transactions(wallet_id);
CREATE INDEX IF NOT EXISTS idx_transactions_user_id   ON transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_ref_id    ON transactions(ref_id);

CREATE TABLE IF NOT EXISTS deposit_requests (
    deposit_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    wallet_id    UUID NOT NULL REFERENCES wallets(wallet_id),
    amount       BIGINT NOT NULL,
    currency     TEXT NOT NULL,
    provider     TEXT NOT NULL,
    provider_ref TEXT NOT NULL UNIQUE,
    status       TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'completed', 'failed', 'reversed')),
    expires_at   TIMESTAMPTZ NOT NULL,
    confirmed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modified_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deposit_requests_user_id   ON deposit_requests(user_id);
CREATE INDEX IF NOT EXISTS idx_deposit_requests_wallet_id ON deposit_requests(wallet_id);

CREATE TABLE IF NOT EXISTS withdrawal_requests (
    withdrawal_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    wallet_id        UUID NOT NULL REFERENCES wallets(wallet_id),
    amount           BIGINT NOT NULL,
    currency         TEXT NOT NULL,
    bank_code        TEXT NOT NULL DEFAULT '',
    account_number   TEXT NOT NULL DEFAULT '',
    account_name     TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'completed', 'failed', 'reversed')),
    provider_ref     TEXT NOT NULL DEFAULT '',
    rejection_reason TEXT NOT NULL DEFAULT '',
    processed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modified_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_withdrawal_requests_user_id   ON withdrawal_requests(user_id);
CREATE INDEX IF NOT EXISTS idx_withdrawal_requests_wallet_id ON withdrawal_requests(wallet_id);
