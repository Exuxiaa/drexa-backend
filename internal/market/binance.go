package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// defaultBinanceBaseURL is Binance's official public market-data mirror. It
// serves the same /api/v3/klines and /api/v3/ticker/24hr endpoints as
// api.binance.com but without auth and without the geo-restrictions that make
// api.binance.com unreachable from many regions. Override with BINANCE_BASE_URL.
const defaultBinanceBaseURL = "https://data-api.binance.vision"

// KlineBar is one OHLCV candlestick in the frontend's expected format.
type KlineBar struct {
	T int64   `json:"t"` // open time (ms)
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`
	V float64 `json:"v"`
}

// TickerStats holds 24-hour market statistics for one pair.
type TickerStats struct {
	Pair  string  `json:"pair"`
	Price float64 `json:"price"`
	Ch    float64 `json:"ch"`   // 24h % change
	Vol   float64 `json:"vol"`  // 24h volume (base asset)
	High  float64 `json:"high"` // 24h high
	Low   float64 `json:"low"`  // 24h low
}

// BinanceClient wraps the Binance REST API.
type BinanceClient struct {
	http    *http.Client
	baseURL string
}

// NewBinanceClient creates a BinanceClient pointed at the public Binance market
// -data mirror (overridable via the BINANCE_BASE_URL env var).
func NewBinanceClient() *BinanceClient {
	base := defaultBinanceBaseURL
	if v := strings.TrimRight(os.Getenv("BINANCE_BASE_URL"), "/"); v != "" {
		base = v
	}
	return &BinanceClient{
		http:    &http.Client{Timeout: 10 * time.Second},
		baseURL: base,
	}
}

// pairToBinanceSymbol converts "BTC_USD"/"BTC_USDC" → "BTCUSDT", etc.
// Returns ("", false) when the pair format is unrecognised.
func pairToBinanceSymbol(pairID string) (string, bool) {
	parts := strings.SplitN(pairID, "_", 2)
	if len(parts) != 2 {
		return "", false
	}
	base, quote := parts[0], parts[1]
	// Binance's deepest liquidity is in USDT. Map USD/USDC quotes to USDT so
	// every pair resolves to a real, liquid Binance market (USDC≈USDT≈$1).
	if quote == "USD" || quote == "USDC" {
		quote = "USDT"
	}
	return base + quote, true
}

// Klines fetches up to `limit` OHLCV bars for the given Binance symbol and interval.
func (b *BinanceClient) Klines(ctx context.Context, symbol, interval string, limit int) ([]KlineBar, error) {
	url := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=%d", b.baseURL, symbol, interval, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance klines: status %d", resp.StatusCode)
	}

	// Each element: [openTime, open, high, low, close, volume, ...]
	var raw [][]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	bars := make([]KlineBar, 0, len(raw))
	for _, r := range raw {
		if len(r) < 6 {
			continue
		}
		var t int64
		if err := json.Unmarshal(r[0], &t); err != nil {
			continue
		}
		o := parseFloat(r[1])
		h := parseFloat(r[2])
		l := parseFloat(r[3])
		c := parseFloat(r[4])
		v := parseFloat(r[5])
		bars = append(bars, KlineBar{T: t, O: o, H: h, L: l, C: c, V: v})
	}
	return bars, nil
}

// binanceTicker24h is the raw Binance 24hr ticker response.
type binanceTicker24h struct {
	Symbol             string `json:"symbol"`
	PriceChangePercent string `json:"priceChangePercent"`
	LastPrice          string `json:"lastPrice"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	Volume             string `json:"volume"`
}

// Ticker24h fetches a single-symbol 24hr ticker. Returns nil when the symbol is not found.
func (b *BinanceClient) Ticker24h(ctx context.Context, symbol string) (*TickerStats, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", b.baseURL, symbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance ticker: status %d for %s", resp.StatusCode, symbol)
	}

	var t binanceTicker24h
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, err
	}
	return &TickerStats{
		Price: atof(t.LastPrice),
		Ch:    atof(t.PriceChangePercent),
		Vol:   atof(t.Volume),
		High:  atof(t.HighPrice),
		Low:   atof(t.LowPrice),
	}, nil
}

// Tickers24h fetches 24hr tickers for multiple symbols in one request.
// Symbols with no data on Binance are silently skipped.
func (b *BinanceClient) Tickers24h(ctx context.Context, symbols []string) (map[string]*TickerStats, error) {
	if len(symbols) == 0 {
		return nil, nil
	}

	// Build JSON array of quoted symbol strings, e.g. ["BTCUSDT","ETHUSDT"]
	quoted := make([]string, len(symbols))
	for i, s := range symbols {
		quoted[i] = `"` + s + `"`
	}
	url := fmt.Sprintf(`%s/api/v3/ticker/24hr?symbols=[%s]`, b.baseURL, strings.Join(quoted, ","))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Binance rejects the whole batch if any single symbol is invalid. Fall
		// back to fetching each symbol on its own so one bad symbol can't blank
		// out every ticker.
		return b.tickersPerSymbol(ctx, symbols), nil
	}

	var tickers []binanceTicker24h
	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return nil, err
	}

	out := make(map[string]*TickerStats, len(tickers))
	for _, t := range tickers {
		out[t.Symbol] = &TickerStats{
			Price: atof(t.LastPrice),
			Ch:    atof(t.PriceChangePercent),
			Vol:   atof(t.Volume),
			High:  atof(t.HighPrice),
			Low:   atof(t.LowPrice),
		}
	}
	return out, nil
}

// tickersPerSymbol fetches each symbol individually, skipping any that fail.
// Used as a resilience fallback when the batch request is rejected.
func (b *BinanceClient) tickersPerSymbol(ctx context.Context, symbols []string) map[string]*TickerStats {
	out := make(map[string]*TickerStats, len(symbols))
	for _, sym := range symbols {
		stats, err := b.Ticker24h(ctx, sym)
		if err != nil || stats == nil {
			continue
		}
		out[sym] = stats
	}
	return out
}

// parseFloat parses a json.RawMessage that is either a number or a quoted string.
func parseFloat(raw json.RawMessage) float64 {
	// Try unquoted number first
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f
	}
	// Binance encodes many price fields as quoted strings
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		f, _ = strconv.ParseFloat(s, 64)
	}
	return f
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
