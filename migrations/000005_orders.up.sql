-- 000005_orders.up.sql
-- CEX orders and trades

CREATE TABLE IF NOT EXISTS orders (
    order_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(user_id),
    pair_id          TEXT NOT NULL REFERENCES trading_pairs(pair_id),
    side             TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
    type             TEXT NOT NULL CHECK (type IN ('market', 'limit')),
    status           TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'open', 'partially_filled', 'filled', 'cancelled')),
    price            NUMERIC(36, 18),
    quantity         NUMERIC(36, 18) NOT NULL,
    filled_quantity  NUMERIC(36, 18) NOT NULL DEFAULT 0,
    locked_amount    NUMERIC(36, 18) NOT NULL DEFAULT 0,
    fee              NUMERIC(36, 18) NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_orders_pair_id ON orders(pair_id);
CREATE INDEX IF NOT EXISTS idx_orders_status  ON orders(status);

CREATE TABLE IF NOT EXISTS trades (
    trade_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pair_id        TEXT NOT NULL REFERENCES trading_pairs(pair_id),
    maker_order_id UUID NOT NULL REFERENCES orders(order_id),
    taker_order_id UUID NOT NULL REFERENCES orders(order_id),
    price          NUMERIC(36, 18) NOT NULL,
    quantity       NUMERIC(36, 18) NOT NULL,
    maker_fee      NUMERIC(36, 18) NOT NULL DEFAULT 0,
    taker_fee      NUMERIC(36, 18) NOT NULL DEFAULT 0,
    executed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trades_pair_id ON trades(pair_id);
CREATE INDEX IF NOT EXISTS idx_trades_executed_at ON trades(executed_at DESC);
