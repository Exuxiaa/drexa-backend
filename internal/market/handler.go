package market

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins for development, you should restrict this in production
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// HandleWebSocket upgrades the HTTP connection to a WebSocket connection
// and registers the client to the hub.
func HandleWebSocket(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("market/handler: Failed to upgrade connection: %v\n", err)
			return
		}

		client := &Client{
			Hub:  hub,
			Conn: conn,
			Send: make(chan []byte, 256),
		}

		client.Hub.Register <- client

		// Start pumping data to and from the client
		go client.WritePump()
		go client.ReadPump()
	}
}

// validIntervals is the set of intervals Binance accepts for klines.
var validIntervals = map[string]bool{
	"1s": true, "1m": true, "3m": true, "5m": true, "15m": true, "30m": true,
	"1h": true, "2h": true, "4h": true, "6h": true, "8h": true, "12h": true,
	"1d": true, "3d": true, "1w": true, "1M": true,
}

// HandleKlines proxies historical OHLCV data from Binance for a given pair and interval.
//
// Query params:
//   - symbol   — pair ID, e.g. "BTC_USD"
//   - interval — e.g. "1m", "15m", "1h", "1d"
//   - limit    — number of bars, 1–1000 (default 120)
func HandleKlines(bc *BinanceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			http.Error(w, `{"error":"symbol is required"}`, http.StatusBadRequest)
			return
		}

		interval := r.URL.Query().Get("interval")
		if interval == "" {
			interval = "1h"
		}
		if !validIntervals[interval] {
			http.Error(w, `{"error":"invalid interval"}`, http.StatusBadRequest)
			return
		}

		limit := 120
		if ls := r.URL.Query().Get("limit"); ls != "" {
			n, err := strconv.Atoi(ls)
			if err != nil || n < 1 || n > 1000 {
				http.Error(w, `{"error":"limit must be 1–1000"}`, http.StatusBadRequest)
				return
			}
			limit = n
		}

		binanceSym, ok := pairToBinanceSymbol(symbol)
		if !ok {
			http.Error(w, `{"error":"invalid symbol format, expected BASE_QUOTE"}`, http.StatusBadRequest)
			return
		}

		bars, err := bc.Klines(r.Context(), binanceSym, interval, limit)
		if err != nil {
			log.Printf("market/klines: %v", err)
			http.Error(w, `{"error":"failed to fetch klines"}`, http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bars)
	}
}

// HandleTickers returns the latest 24hr ticker stats for all active pairs.
func HandleTickers(cache *TickerCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := cache.All()
		if stats == nil {
			stats = []TickerStats{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}
