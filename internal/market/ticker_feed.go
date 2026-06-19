package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const tickerFeedInterval = 3 * time.Second
const binanceTickerURL = "https://data-api.binance.vision/api/v3/ticker/24hr"

type tickerMessage struct {
	Type      string  `json:"type"`
	Pair      string  `json:"pair"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change_24h"`
	Volume24h float64 `json:"volume_24h"`
	High24h   float64 `json:"high_24h"`
	Low24h    float64 `json:"low_24h"`
}

type binanceTicker struct {
	Symbol             string `json:"symbol"`
	LastPrice          string `json:"lastPrice"`
	PriceChangePercent string `json:"priceChangePercent"`
	Volume             string `json:"volume"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
}

// TickerFeed periodically fetches 24h stats from Binance for all active pairs
// and broadcasts ticker events to the WebSocket hub alongside orderbook events.
type TickerFeed struct {
	hub    *Hub
	pairs  PairLister
	client *http.Client
}

func NewTickerFeed(hub *Hub, pairs PairLister) *TickerFeed {
	return &TickerFeed{
		hub:    hub,
		pairs:  pairs,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (f *TickerFeed) Run(ctx context.Context) {
	ticker := time.NewTicker(tickerFeedInterval)
	defer ticker.Stop()
	log.Println("market/ticker: feed started (source: Binance 24hr)")
	for {
		select {
		case <-ctx.Done():
			log.Println("market/ticker: feed stopped")
			return
		case <-ticker.C:
			f.publish(ctx)
		}
	}
}

func (f *TickerFeed) publish(ctx context.Context) {
	pairIDs, err := f.pairs.ActivePairIDs(ctx)
	if err != nil {
		log.Printf("market/ticker: list pairs: %v", err)
		return
	}
	if len(pairIDs) == 0 {
		return
	}

	// Build the symbols array parameter, e.g. ["BTCUSDC","ETHUSDC"]
	syms := make([]string, 0, len(pairIDs))
	symToPair := make(map[string]string, len(pairIDs))
	for _, pid := range pairIDs {
		sym := PairToSymbol(pid)
		syms = append(syms, `"`+sym+`"`)
		symToPair[sym] = pid
	}
	symParam := "[" + strings.Join(syms, ",") + "]"

	url := fmt.Sprintf("%s?symbols=%s", binanceTickerURL, symParam)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("market/ticker: build request: %v", err)
		return
	}

	resp, err := f.client.Do(req)
	if err != nil {
		log.Printf("market/ticker: fetch: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tickers []binanceTicker
	if err := json.Unmarshal(body, &tickers); err != nil {
		log.Printf("market/ticker: decode: %v", err)
		return
	}

	for _, t := range tickers {
		pairID, ok := symToPair[t.Symbol]
		if !ok {
			continue
		}
		msg := tickerMessage{
			Type:      "ticker",
			Pair:      pairID,
			Price:     parseFloatStr(t.LastPrice),
			Change24h: parseFloatStr(t.PriceChangePercent),
			Volume24h: parseFloatStr(t.Volume),
			High24h:   parseFloatStr(t.HighPrice),
			Low24h:    parseFloatStr(t.LowPrice),
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			log.Printf("market/ticker: marshal %s: %v", pairID, err)
			continue
		}
		f.hub.Broadcast <- payload
	}
}

// PairToSymbol converts a pair ID like "BTC_USDC" into a Binance symbol "BTCUSDC".
func PairToSymbol(pairID string) string {
	return strings.ReplaceAll(pairID, "_", "")
}

func parseFloatStr(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
