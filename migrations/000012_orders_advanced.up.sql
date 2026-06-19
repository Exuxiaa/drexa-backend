-- 000012_orders_advanced.up.sql
-- Advanced order types: stop-limit (dormant until a trigger price is reached)
-- and OCO (one-cancels-the-other, stored as a linked limit + stop-limit pair).

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS stop_price   NUMERIC(36, 18),
    ADD COLUMN IF NOT EXISTS oco_group_id UUID;

-- Allow the new 'stop-limit' order type. OCO is not a stored type: it expands
-- into a 'limit' leg + a 'stop-limit' leg sharing one oco_group_id.
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_type_check;
ALTER TABLE orders ADD CONSTRAINT orders_type_check
    CHECK (type IN ('market', 'limit', 'stop-limit'));

-- 'untriggered' is the dormant state of a stop order waiting for its stop price.
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check
    CHECK (status IN ('pending', 'open', 'partially_filled', 'filled', 'cancelled', 'untriggered'));

CREATE INDEX IF NOT EXISTS idx_orders_oco_group ON orders(oco_group_id);
-- Speeds up reloading dormant stop orders into the in-memory trigger book on boot.
CREATE INDEX IF NOT EXISTS idx_orders_untriggered ON orders(status) WHERE status = 'untriggered';
