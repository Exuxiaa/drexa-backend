-- Remove the *_USDT pairs this migration added. Leave the coins (USDT/BTC/ETH and
-- friends) in place: other pairs (e.g. *_IDR) and balances reference them, and the
-- coin rows are harmless to keep.
DELETE FROM trading_pairs WHERE pair_id IN
    ('BTC_USDT', 'ETH_USDT', 'BNB_USDT', 'SOL_USDT',
     'XRP_USDT', 'ADA_USDT', 'AVAX_USDT', 'LINK_USDT', 'DOGE_USDT');
