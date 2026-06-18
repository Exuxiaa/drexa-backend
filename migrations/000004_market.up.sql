-- 000004_market.up.sql
-- Coins, trading pairs, price snapshots, candles

CREATE TABLE IF NOT EXISTS coins (
    coin_id    TEXT PRIMARY KEY,
    symbol     TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    decimals   INTEGER NOT NULL DEFAULT 18,
    network    TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active'
                   CHECK (status IN ('active', 'suspended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS trading_pairs (
    pair_id               TEXT PRIMARY KEY,
    base_coin             TEXT NOT NULL REFERENCES coins(coin_id),
    quote_coin            TEXT NOT NULL REFERENCES coins(coin_id),
    status                TEXT NOT NULL DEFAULT 'active'
                              CHECK (status IN ('active', 'suspended')),
    min_order_size        NUMERIC(36, 18) NOT NULL DEFAULT 0,
    price_decimal_places  INTEGER NOT NULL DEFAULT 2,
    UNIQUE (base_coin, quote_coin)
);

CREATE TABLE IF NOT EXISTS price_snapshots (
    snapshot_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pair_id     TEXT NOT NULL REFERENCES trading_pairs(pair_id),
    price       NUMERIC(36, 18) NOT NULL,
    change_24h  NUMERIC(10, 4) NOT NULL DEFAULT 0,
    volume_24h  NUMERIC(36, 18) NOT NULL DEFAULT 0,
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_price_snapshots_pair_id  ON price_snapshots(pair_id);
CREATE INDEX IF NOT EXISTS idx_price_snapshots_time     ON price_snapshots(timestamp DESC);

CREATE TABLE IF NOT EXISTS candles (
    candle_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pair_id    TEXT NOT NULL REFERENCES trading_pairs(pair_id),
    interval   TEXT NOT NULL CHECK (interval IN ('1m', '5m', '1h', '1d')),
    open       NUMERIC(36, 18) NOT NULL,
    high       NUMERIC(36, 18) NOT NULL,
    low        NUMERIC(36, 18) NOT NULL,
    close      NUMERIC(36, 18) NOT NULL,
    volume     NUMERIC(36, 18) NOT NULL,
    open_time  TIMESTAMPTZ NOT NULL,
    close_time TIMESTAMPTZ NOT NULL,
    UNIQUE (pair_id, interval, open_time)
);

CREATE INDEX IF NOT EXISTS idx_candles_pair_interval ON candles(pair_id, interval, open_time DESC);
