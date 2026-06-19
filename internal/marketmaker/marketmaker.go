// Package marketmaker provides resting liquidity to the internal matching engine.
//
// The internal order book starts empty, so without a counterparty a user's market
// order has nothing to hit and a limit order rests forever. This bot acts as a
// "house" liquidity provider: a dedicated system account that, on every tick,
// refreshes a resting buy and a resting sell limit order around the live Binance
// price for each *_USDT pair. User orders then match against the house and settle
// through the normal ledger path (order.Service -> SettleTrade).
//
// The house is intentionally an effectively-infinite counterparty: before quoting,
// its inventory is topped up to a target, so a demo never runs dry. This is a
// liquidity convenience for the project, not a real risk-managed market maker.
package marketmaker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"drexa/internal/market"
	"drexa/internal/order"
)

// maxSafeUnits keeps every smallest-unit amount well under math.MaxInt64
// (~9.22e18). ETH/BNB/AVAX/LINK are stored at 1e18, so a single balance or order
// amount for them cannot exceed ~9 whole units; we clamp to 8e18 for headroom.
const maxSafeUnits = 8_000_000_000_000_000_000

// PairLister yields the currently active trading pair IDs (e.g. "BTC_USDT").
// Satisfied by the same adapter the market feeds use.
type PairLister interface {
	ActivePairIDs(ctx context.Context) ([]string, error)
}

// Treasury tops up the house account's inventory so it can always quote.
type Treasury interface {
	// EnsureBalance makes the house hold at least target smallest-units of
	// currency, crediting the difference if it is short.
	EnsureBalance(ctx context.Context, currency string, target int64) error
}

// Bot maintains two-sided resting liquidity for every *_USDT pair.
type Bot struct {
	orders   order.Service
	pairs    PairLister
	treasury Treasury
	client   *http.Client

	houseID  string
	spread   float64       // half-spread, e.g. 0.001 = 10 bps each side
	notional float64       // target USDT notional per side per pair
	interval time.Duration

	// resting tracks the house's current resting order IDs per pair so they can
	// be cancelled and replaced each tick: pairID -> [buyID, sellID].
	resting map[string][2]string
}

// New builds a market-maker bot. houseID must be the user_id of a pre-created
// system account that owns the house wallets.
func New(orders order.Service, pairs PairLister, treasury Treasury, houseID string) *Bot {
	return &Bot{
		orders:   orders,
		pairs:    pairs,
		treasury: treasury,
		client:   &http.Client{Timeout: 5 * time.Second},
		houseID:  houseID,
		spread:   0.001,
		notional: 25_000,
		interval: 5 * time.Second,
		resting:  make(map[string][2]string),
	}
}

// Run quotes continuously until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	log.Info().Str("house", b.houseID).Msg("marketmaker: started")
	// Quote once immediately so liquidity is present without waiting a full tick.
	b.quote(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("marketmaker: stopped")
			return
		case <-ticker.C:
			b.quote(ctx)
		}
	}
}

func (b *Bot) quote(ctx context.Context) {
	pairIDs, err := b.pairs.ActivePairIDs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("marketmaker: list pairs")
		return
	}

	// Only make markets on USDT-quoted pairs — balances are USDT and Binance
	// quotes these symbols (e.g. BTCUSDT). Skip *_IDR and similar.
	usdtPairs := make([]string, 0, len(pairIDs))
	for _, pid := range pairIDs {
		if strings.HasSuffix(pid, "_USDT") {
			usdtPairs = append(usdtPairs, pid)
		}
	}
	if len(usdtPairs) == 0 {
		return
	}

	prices, err := b.fetchPrices(ctx, usdtPairs)
	if err != nil {
		log.Error().Err(err).Msg("marketmaker: fetch prices")
		return
	}

	for _, pid := range usdtPairs {
		px, ok := prices[pid]
		if !ok || px <= 0 {
			continue
		}
		b.requote(ctx, pid, px)
	}
}

// requote cancels the house's previous resting orders for a pair and posts fresh
// ones around px.
func (b *Bot) requote(ctx context.Context, pairID string, px float64) {
	base, _, ok := strings.Cut(pairID, "_")
	if !ok {
		return
	}

	// Cancel last round's quotes (ignore errors: they may have filled or expired).
	if prev, ok := b.resting[pairID]; ok {
		for _, oid := range prev {
			if oid != "" {
				_, _ = b.orders.CancelOrder(ctx, b.houseID, oid)
			}
		}
		delete(b.resting, pairID)
	}

	// Size each side to the target USDT notional, clamped so the base-coin
	// amount can't overflow int64 at its smallest-unit scale.
	qty := b.notional / px
	if cap := maxQty(base); qty > cap {
		qty = cap
	}
	if qty <= 0 {
		return
	}

	// Top up inventory so the lock at order-placement time always succeeds:
	// the sell locks base coin (qty), the buy locks USDT (qty*px).
	if err := b.treasury.EnsureBalance(ctx, base, houseTarget(base)); err != nil {
		log.Error().Err(err).Str("currency", base).Msg("marketmaker: top up base")
		return
	}
	if err := b.treasury.EnsureBalance(ctx, "USDT", houseTarget("USDT")); err != nil {
		log.Error().Err(err).Msg("marketmaker: top up USDT")
		return
	}

	buyPx := px * (1 - b.spread)
	sellPx := px * (1 + b.spread)

	var slot [2]string
	if o, err := b.place(ctx, pairID, order.SideBuy, qty, buyPx); err != nil {
		b.logPlaceErr(err, pairID, order.SideBuy)
	} else {
		slot[0] = o.OrderID
	}
	if o, err := b.place(ctx, pairID, order.SideSell, qty, sellPx); err != nil {
		b.logPlaceErr(err, pairID, order.SideSell)
	} else {
		slot[1] = o.OrderID
	}
	if slot[0] != "" || slot[1] != "" {
		b.resting[pairID] = slot
	}
}

func (b *Bot) place(ctx context.Context, pairID string, side order.OrderSide, qty, price float64) (*order.Order, error) {
	p := price
	return b.orders.CreateOrder(ctx, b.houseID, order.OrderRequest{
		PairID:   pairID,
		Side:     side,
		Type:     order.TypeLimit,
		Quantity: qty,
		Price:    &p,
	})
}

// logPlaceErr keeps expected, benign rejections quiet (e.g. a quote below the
// pair's minimum size) while surfacing anything unexpected.
func (b *Bot) logPlaceErr(err error, pairID string, side order.OrderSide) {
	switch err {
	case order.ErrBelowMinOrderSize:
		return
	default:
		log.Warn().Err(err).Str("pair", pairID).Str("side", string(side)).Msg("marketmaker: place quote")
	}
}

const binancePriceURL = "https://data-api.binance.vision/api/v3/ticker/price"

// fetchPrices returns the latest Binance price keyed by pair ID for the given pairs.
func (b *Bot) fetchPrices(ctx context.Context, pairIDs []string) (map[string]float64, error) {
	syms := make([]string, 0, len(pairIDs))
	symToPair := make(map[string]string, len(pairIDs))
	for _, pid := range pairIDs {
		sym := market.PairToSymbol(pid)
		syms = append(syms, `"`+sym+`"`)
		symToPair[sym] = pid
	}
	url := fmt.Sprintf("%s?symbols=[%s]", binancePriceURL, strings.Join(syms, ","))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var rows []struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode prices: %w (body: %.120s)", err, string(body))
	}

	out := make(map[string]float64, len(rows))
	for _, r := range rows {
		if pid, ok := symToPair[r.Symbol]; ok {
			var f float64
			fmt.Sscanf(r.Price, "%g", &f)
			out[pid] = f
		}
	}
	return out, nil
}

// smallestUnitFactor mirrors order.smallestUnitFactor: base units per whole unit.
func smallestUnitFactor(currency string) int64 {
	switch currency {
	case "BTC", "DOGE":
		return 100_000_000
	case "ETH", "BNB", "AVAX", "LINK":
		return 1_000_000_000_000_000_000
	case "SOL":
		return 1_000_000_000
	case "USDT", "XRP", "ADA":
		return 1_000_000
	default: // USD, IDR
		return 100
	}
}

// maxQty caps an order quantity so qty * factor stays within int64.
func maxQty(currency string) float64 {
	return float64(maxSafeUnits) / float64(smallestUnitFactor(currency))
}

// houseTarget is the inventory level the house is kept topped up to, in smallest
// units. Clamped to maxSafeUnits for 1e18-scaled coins to avoid int64 overflow.
func houseTarget(currency string) int64 {
	f := smallestUnitFactor(currency)
	if f >= 1_000_000_000_000_000_000 {
		return maxSafeUnits
	}
	return 1_000_000_000_000_000 // 1e15 base units — huge for sub-1e18 coins
}
