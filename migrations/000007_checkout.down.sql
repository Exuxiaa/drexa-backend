DROP TABLE IF EXISTS purchases;

ALTER TABLE users DROP COLUMN IF EXISTS stripe_customer_id;
