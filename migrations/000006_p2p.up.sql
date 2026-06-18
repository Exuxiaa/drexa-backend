-- 000006_p2p.up.sql
-- P2P marketplace: advertisements, orders, disputes

CREATE TABLE IF NOT EXISTS p2p_advertisements (
    advertisement_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id        UUID NOT NULL REFERENCES users(user_id),
    pair_id          TEXT NOT NULL REFERENCES trading_pairs(pair_id),
    price            NUMERIC(36, 18) NOT NULL,
    min_amount       NUMERIC(36, 18) NOT NULL,
    max_amount       NUMERIC(36, 18) NOT NULL,
    payment_method   TEXT NOT NULL,
    payment_window   INTEGER NOT NULL,   -- minutes the buyer has to complete payment
    status           TEXT NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'paused', 'completed')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_p2p_ads_seller_id ON p2p_advertisements(seller_id);
CREATE INDEX IF NOT EXISTS idx_p2p_ads_status    ON p2p_advertisements(status);

CREATE TABLE IF NOT EXISTS p2p_orders (
    p2p_order_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    advertisement_id UUID NOT NULL REFERENCES p2p_advertisements(advertisement_id),
    buyer_id         UUID NOT NULL REFERENCES users(user_id),
    seller_id        UUID NOT NULL REFERENCES users(user_id),
    amount           NUMERIC(36, 18) NOT NULL,
    total_idr        NUMERIC(20, 2) NOT NULL,
    status           TEXT NOT NULL DEFAULT 'created'
                         CHECK (status IN ('created', 'paid', 'released', 'disputed', 'cancelled')),
    payment_proof_url TEXT,
    escrow_wallet_id UUID NOT NULL REFERENCES wallets(wallet_id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at          TIMESTAMPTZ,
    released_at      TIMESTAMPTZ,
    expired_at       TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_p2p_orders_buyer_id  ON p2p_orders(buyer_id);
CREATE INDEX IF NOT EXISTS idx_p2p_orders_seller_id ON p2p_orders(seller_id);

CREATE TABLE IF NOT EXISTS p2p_disputes (
    p2p_dispute_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    p2p_order_id   UUID NOT NULL REFERENCES p2p_orders(p2p_order_id),
    raised_by      UUID NOT NULL REFERENCES users(user_id),
    reason         TEXT NOT NULL,
    evidence_url   TEXT,
    status         TEXT NOT NULL DEFAULT 'open'
                       CHECK (status IN ('open', 'resolved')),
    resolved_by    UUID REFERENCES users(user_id),
    resolution     TEXT NOT NULL DEFAULT '',
    resolved_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_p2p_disputes_order_id ON p2p_disputes(p2p_order_id);
