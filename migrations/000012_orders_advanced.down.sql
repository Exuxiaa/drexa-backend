-- 000012_orders_advanced.down.sql

DROP INDEX IF EXISTS idx_orders_untriggered;
DROP INDEX IF EXISTS idx_orders_oco_group;

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check
    CHECK (status IN ('pending', 'open', 'partially_filled', 'filled', 'cancelled'));

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_type_check;
ALTER TABLE orders ADD CONSTRAINT orders_type_check
    CHECK (type IN ('market', 'limit'));

ALTER TABLE orders
    DROP COLUMN IF EXISTS oco_group_id,
    DROP COLUMN IF EXISTS stop_price;
