package market

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const binanceKlinesURL = "https://data-api.binance.vision/api/v3/klines"

var validKlineIntervals = map[string]bool{
	"1m": true, "5m": true, "15m": true,
	"1h": true, "4h": true, "1d": true, "1w": true,
}

type candlePoint struct {
	T int64   `json:"t"` // open time ms
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`
	V float64 `json:"v"`
}

type klinesResponse struct {
	Pair     string        `json:"pair"`
	Interval string        `json:"interval"`
	Candles  []candlePoint `json:"candles"`
}

// HandleKlines proxies candlestick data from Binance for a given pair and
// interval, converting the pair ID format (BTC_USDC) to Binance symbol
// (BTCUSDC) transparently. Public — no auth required.
// Route: GET /api/v1/market/klines/{pairID}?interval=1h&limit=120
func HandleKlines() http.Handler {
	client := &http.Client{Timeout: 10 * time.Second}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pairID := r.PathValue("pairID")
		if pairID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pairID is required"})
			return
		}

		interval := r.URL.Query().Get("interval")
		if !validKlineIntervals[interval] {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "interval must be one of: 1m, 5m, 15m, 1h, 4h, 1d, 1w",
			})
			return
		}

		limit := 120
		if q := r.URL.Query().Get("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 1000 {
				limit = n
			}
		}

		sym := PairToSymbol(pairID)
		url := fmt.Sprintf("%s?symbol=%s&interval=%s&limit=%d", binanceKlinesURL, sym, interval, limit)

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}

		resp, err := client.Do(req)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch market data"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": fmt.Sprintf("upstream error: %s", string(body)),
			})
			return
		}

		body, _ := io.ReadAll(resp.Body)
		// Binance klines response is [][]json.RawMessage
		var raw [][]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to parse market data"})
			return
		}

		candles := make([]candlePoint, 0, len(raw))
		for _, row := range raw {
			if len(row) < 6 {
				continue
			}
			var openTime int64
			if err := json.Unmarshal(row[0], &openTime); err != nil {
				continue
			}
			candles = append(candles, candlePoint{
				T: openTime,
				O: parseKlineField(row[1]),
				H: parseKlineField(row[2]),
				L: parseKlineField(row[3]),
				C: parseKlineField(row[4]),
				V: parseKlineField(row[5]),
			})
		}

		writeJSON(w, http.StatusOK, klinesResponse{
			Pair:     pairID,
			Interval: interval,
			Candles:  candles,
		})
	})
}

// parseKlineField parses a Binance kline field which is a JSON string like "65432.50".
func parseKlineField(b json.RawMessage) float64 {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}
