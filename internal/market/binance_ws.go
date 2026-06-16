package market

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const binanceWSURL = "wss://data-stream.binance.vision/ws/!miniTicker@arr"

// BinanceWSClient manages the connection to Binance and forwards messages to the Hub
type BinanceWSClient struct {
	hub *Hub
}

func NewBinanceWSClient(hub *Hub) *BinanceWSClient {
	return &BinanceWSClient{
		hub: hub,
	}
}

func (b *BinanceWSClient) Run() {
	for {
		b.connectAndListen()
		log.Println("market/binance: connection lost, reconnecting in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
}

func (b *BinanceWSClient) connectAndListen() {
	log.Printf("market/binance: connecting to %s\n", binanceWSURL)
	conn, _, err := websocket.DefaultDialer.Dial(binanceWSURL, nil)
	if err != nil {
		log.Printf("market/binance: dial error: %v", err)
		return
	}
	defer conn.Close()

	log.Println("market/binance: connected successfully")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("market/binance: read error: %v", err)
			return // return to trigger reconnect
		}

		// Forward the raw JSON message directly to the hub
		b.hub.Broadcast <- message
	}
}
