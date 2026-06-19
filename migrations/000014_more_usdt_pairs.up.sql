-- Seed every coin + USDT trading pair the frontend trade page offers. The trade
-- ticket builds pair_id as `${coin.sym}_USDT` for every coin in COINS (BTC, ETH,
-- SOL, BNB, XRP, ADA, AVAX, LINK, DOGE); without these rows those orders 404 with
-- ErrPairNotFound.
--
-- This migration is fully self-contained: it inserts USDT and all base coins
-- itself (instead of assuming 000001's seed ran) because this database was
-- force-pointed past the rewritten 000001 and only had BTC/ETH/USD. trading_pairs
-- has FKs to coins(coin_id), so the coins must exist before the pairs. Everything
-- is ON CONFLICT DO NOTHING, so it is safe to re-run.

INSERT INTO coins (coin_id, symbol, name, decimals, network, status)
VALUES
    ('USDT', 'USDT', 'Tether USD', 6,  'Ethereum',            'active'),
    ('BTC',  'BTC',  'Bitcoin',    8,  'Bitcoin',             'active'),
    ('ETH',  'ETH',  'Ethereum',   18, 'Ethereum',            'active'),
    ('BNB',  'BNB',  'BNB',        18, 'Binance Smart Chain', 'active'),
    ('SOL',  'SOL',  'Solana',     9,  'Solana',              'active'),
    ('XRP',  'XRP',  'XRP',        6,  'XRP Ledger',          'active'),
    ('ADA',  'ADA',  'Cardano',    6,  'Cardano',             'active'),
    ('AVAX', 'AVAX', 'Avalanche',  18, 'Avalanche',           'active'),
    ('LINK', 'LINK', 'Chainlink',  18, 'Ethereum',            'active'),
    ('DOGE', 'DOGE', 'Dogecoin',   8,  'Dogecoin',            'active')
ON CONFLICT (coin_id) DO NOTHING;

INSERT INTO trading_pairs (pair_id, base_coin, quote_coin, status, min_order_size, price_decimal_places)
VALUES
    ('BTC_USDT',  'BTC',  'USDT', 'active', 0.0001, 2),
    ('ETH_USDT',  'ETH',  'USDT', 'active', 0.001,  2),
    ('BNB_USDT',  'BNB',  'USDT', 'active', 0.01,   2),
    ('SOL_USDT',  'SOL',  'USDT', 'active', 0.1,    2),
    ('XRP_USDT',  'XRP',  'USDT', 'active', 1.0,    4),
    ('ADA_USDT',  'ADA',  'USDT', 'active', 1.0,    4),
    ('AVAX_USDT', 'AVAX', 'USDT', 'active', 0.01,   2),
    ('LINK_USDT', 'LINK', 'USDT', 'active', 0.1,    2),
    ('DOGE_USDT', 'DOGE', 'USDT', 'active', 1.0,    5)
ON CONFLICT (pair_id) DO NOTHING;
