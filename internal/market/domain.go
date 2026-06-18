package market

import (
	"errors"
	"time"
)

// ─── Enums ───────────────────────────────────────────────────────────────────

type AssetStatus string

const (
	StatusActive    AssetStatus = "active"
	StatusSuspended AssetStatus = "suspended"
)

type CandleInterval string

const (
	Interval1m  CandleInterval = "1m"
	Interval5m  CandleInterval = "5m"
	Interval1h  CandleInterval = "1h"
	Interval1d  CandleInterval = "1d"
)

// ─── Entities ────────────────────────────────────────────────────────────────

// Coin is a master-data record for a supported cryptocurrency.
type Coin struct {
	CoinID    string      `gorm:"primaryKey;column:coin_id"` // e.g. "bitcoin"
	Symbol    string      `gorm:"column:symbol;uniqueIndex"` // e.g. "BTC"
	Name      string      `gorm:"column:name"`
	Decimals  int         `gorm:"column:decimals;default:18"`
	Network   string      `gorm:"column:network"` // e.g. "ERC20", "native"
	Status    AssetStatus `gorm:"column:status;default:active"`
	CreatedAt time.Time   `gorm:"column:created_at;autoCreateTime"`
}

// TradingPair defines a tradeable instrument (e.g. BTC/IDR).
type TradingPair struct {
	PairID              string      `gorm:"primaryKey;column:pair_id"` // e.g. "BTC_IDR"
	BaseCoin            string      `gorm:"column:base_coin"`          // FK to coins
	QuoteCoin           string      `gorm:"column:quote_coin"`         // FK to coins
	Status              AssetStatus `gorm:"column:status;default:active"`
	MinOrderSize        float64     `gorm:"column:min_order_size;type:numeric(36,18);default:0"`
	PriceDecimalPlaces  int         `gorm:"column:price_decimal_places;default:2"`
}

// PriceSnapshot is a point-in-time market summary for a trading pair.
type PriceSnapshot struct {
	SnapshotID string    `gorm:"primaryKey;column:snapshot_id"`
	PairID     string    `gorm:"column:pair_id;index"`
	Price      float64   `gorm:"column:price;type:numeric(36,18)"`
	Change24h  float64   `gorm:"column:change_24h;type:numeric(10,4);default:0"`
	Volume24h  float64   `gorm:"column:volume_24h;type:numeric(36,18);default:0"`
	Timestamp  time.Time `gorm:"column:timestamp;index"`
}

// Candle stores OHLCV data for charting.
type Candle struct {
	CandleID  string         `gorm:"primaryKey;column:candle_id"`
	PairID    string         `gorm:"column:pair_id;index"`
	Interval  CandleInterval `gorm:"column:interval"`
	Open      float64        `gorm:"column:open;type:numeric(36,18)"`
	High      float64        `gorm:"column:high;type:numeric(36,18)"`
	Low       float64        `gorm:"column:low;type:numeric(36,18)"`
	Close     float64        `gorm:"column:close;type:numeric(36,18)"`
	Volume    float64        `gorm:"column:volume;type:numeric(36,18)"`
	OpenTime  time.Time      `gorm:"column:open_time"`
	CloseTime time.Time      `gorm:"column:close_time"`
}

// ─── Domain Errors ───────────────────────────────────────────────────────────

var (
	ErrCoinNotFound = errors.New("coin not found")
	ErrPairNotFound = errors.New("trading pair not found")
	ErrPairSuspended = errors.New("trading pair is suspended")
)
