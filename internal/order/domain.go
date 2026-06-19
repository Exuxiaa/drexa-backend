package order

import (
	"context"
	"errors"
	"time"

	"drexa/internal/matching"
)

// ─── Enums ───────────────────────────────────────────────────────────────────

type OrderSide string

const (
	SideBuy  OrderSide = "buy"
	SideSell OrderSide = "sell"
)

type OrderType string

const (
	TypeMarket    OrderType = "market"
	TypeLimit     OrderType = "limit"
	TypeStopLimit OrderType = "stop-limit"
	// TypeOCO is a request-only pseudo type. An OCO order is never stored as a
	// single row: it expands into a 'limit' leg + a 'stop-limit' leg that share
	// one oco_group_id, so that a fill/trigger of either leg cancels the other.
	TypeOCO OrderType = "oco"
)

type OrderStatus string

const (
	StatusPending         OrderStatus = "pending"
	StatusOpen            OrderStatus = "open"
	StatusPartiallyFilled OrderStatus = "partially_filled"
	StatusFilled          OrderStatus = "filled"
	StatusCancelled       OrderStatus = "cancelled"
	// StatusUntriggered is the dormant state of a stop order whose stop price
	// has not yet been reached. It rests off-book in the in-memory trigger book.
	StatusUntriggered OrderStatus = "untriggered"
)

// OrderStatusFilter selects which lifecycle bucket ListOrders returns.
type OrderStatusFilter string

const (
	FilterAll    OrderStatusFilter = "all"
	FilterOpen   OrderStatusFilter = "open"   // active: pending/open/partially_filled/untriggered
	FilterClosed OrderStatusFilter = "closed" // terminal: filled/cancelled
)

// OrderFilter narrows a user's order listing.
type OrderFilter struct {
	PairID string // optional; empty means all pairs
	Status OrderStatusFilter
	Limit  int // <= 0 means no limit
}

// ─── Entities ────────────────────────────────────────────────────────────────

// Order is a user's intent to buy or sell a trading pair.
type Order struct {
	OrderID        string      `gorm:"primaryKey;column:order_id" json:"order_id"`
	UserID         string      `gorm:"column:user_id;index" json:"user_id"`
	PairID         string      `gorm:"column:pair_id;index" json:"pair_id"`
	Side           OrderSide   `gorm:"column:side" json:"side"`
	Type           OrderType   `gorm:"column:type" json:"type"`
	Status         OrderStatus `gorm:"column:status;default:pending" json:"status"`
	Price          *float64    `gorm:"column:price;type:numeric(36,18)" json:"price,omitempty"` // nil for market orders
	// StopPrice is the trigger price for stop-limit orders; nil otherwise.
	StopPrice      *float64 `gorm:"column:stop_price;type:numeric(36,18)" json:"stop_price,omitempty"`
	Quantity       float64  `gorm:"column:quantity;type:numeric(36,18)" json:"quantity"`
	FilledQuantity float64  `gorm:"column:filled_quantity;type:numeric(36,18);default:0" json:"filled_quantity"`
	LockedAmount   float64  `gorm:"column:locked_amount;type:numeric(36,18);default:0" json:"locked_amount"`
	Fee            float64  `gorm:"column:fee;type:numeric(36,18);default:0" json:"fee"`
	// OCOGroupID links the two legs of an OCO order; nil for standalone orders.
	OCOGroupID *string   `gorm:"column:oco_group_id" json:"oco_group_id,omitempty"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// Trade is the immutable record produced when two orders match.
type Trade struct {
	TradeID      string    `gorm:"primaryKey;column:trade_id" json:"trade_id"`
	PairID       string    `gorm:"column:pair_id;index" json:"pair_id"`
	MakerOrderID string    `gorm:"column:maker_order_id" json:"maker_order_id"`
	TakerOrderID string    `gorm:"column:taker_order_id" json:"taker_order_id"`
	Price        float64   `gorm:"column:price;type:numeric(36,18)" json:"price"`
	Quantity     float64   `gorm:"column:quantity;type:numeric(36,18)" json:"quantity"`
	MakerFee     float64   `gorm:"column:maker_fee;type:numeric(36,18);default:0" json:"maker_fee"`
	TakerFee     float64   `gorm:"column:taker_fee;type:numeric(36,18);default:0" json:"taker_fee"`
	ExecutedAt   time.Time `gorm:"column:executed_at;autoCreateTime" json:"executed_at"`
}

// TradeView is a trade enriched with the requesting user's perspective: which
// side they were on and whether they were the maker or taker. Returned by the
// per-user trade-history endpoint.
type TradeView struct {
	TradeID    string    `json:"trade_id"`
	PairID     string    `json:"pair_id"`
	OrderID    string    `json:"order_id"`
	Side       OrderSide `json:"side"`
	Role       string    `json:"role"` // "maker" or "taker"
	Price      float64   `json:"price"`
	Quantity   float64   `json:"quantity"`
	Fee        float64   `json:"fee"`
	ExecutedAt time.Time `json:"executed_at"`
}

// ─── Service & Repository Interfaces ─────────────────────────────────────────

// Service is the order domain's business-logic entrypoint.
type Service interface {
	CreateOrder(ctx context.Context, userID string, req OrderRequest) (*Order, error)
	CancelOrder(ctx context.Context, userID, orderID string) (*Order, error)
	// ListOrders returns the caller's orders, newest first, narrowed by filter.
	ListOrders(ctx context.Context, userID string, f OrderFilter) ([]Order, error)
	// ListTrades returns the caller's executed fills, newest first.
	ListTrades(ctx context.Context, userID string, limit int) ([]TradeView, error)
	// OrderBookDepth returns the live aggregated book for a pair in real
	// (float) prices, best prices first. maxLevels <= 0 returns every level.
	OrderBookDepth(ctx context.Context, pairID string, maxLevels int) (*OrderBookSnapshot, error)
	// RestoreStops rehydrates the in-memory stop-trigger book from any dormant
	// (untriggered) orders persisted in the database. Call once on startup.
	RestoreStops(ctx context.Context) error
}

// TradeEvent is emitted after a match prints one or more trades, carrying the
// last executed price for a pair. The market ticker feed consumes it to publish
// a real-time, trade-driven ticker over the WebSocket. Decoupled from the market
// domain: the observer is supplied as a plain callback in cmd/server.
type TradeEvent struct {
	PairID   string
	Price    float64 // last executed price
	Quantity float64 // total quantity traded this match
}

// TradeObserver is notified after each match that produces trades. Optional;
// a nil observer disables trade-driven ticker updates.
type TradeObserver func(TradeEvent)

// OrderBookLevel is one aggregated price level: total resting quantity at a price.
type OrderBookLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

// OrderBookSnapshot is a point-in-time view of a pair's book, best prices first
// (bids highest first, asks lowest first). Version is the engine's mutation
// counter at snapshot time, for gap detection and update de-duplication.
type OrderBookSnapshot struct {
	PairID  string           `json:"pair_id"`
	Version uint64           `json:"version"`
	Bids    []OrderBookLevel `json:"bids"`
	Asks    []OrderBookLevel `json:"asks"`
}

// Repository persists orders and trades.
type Repository interface {
	Create(ctx context.Context, o *Order) error
	FindByID(ctx context.Context, orderID string) (*Order, error)
	FindByUserID(ctx context.Context, userID string) ([]Order, error)
	// FindOrders returns a user's orders, newest first, narrowed by filter.
	FindOrders(ctx context.Context, userID string, f OrderFilter) ([]Order, error)
	// FindUntriggered returns every dormant stop order across all users, for
	// rehydrating the in-memory trigger book on startup.
	FindUntriggered(ctx context.Context) ([]Order, error)
	// FindByOCOGroup returns both legs of an OCO group.
	FindByOCOGroup(ctx context.Context, groupID string) ([]Order, error)
	// FindTradesByUserID returns the caller's fills (newest first), annotated
	// with their side and maker/taker role. limit <= 0 means no limit.
	FindTradesByUserID(ctx context.Context, userID string, limit int) ([]TradeView, error)
	// Update persists mutable order fields (status, filled_quantity, fee).
	Update(ctx context.Context, o *Order) error
	// SaveTrades persists the fills produced by a match, atomically.
	SaveTrades(ctx context.Context, trades []Trade) error
}

// Matcher is the narrow interface the order service needs from the in-memory
// matching engine. Satisfied by *matching.Engine, wired in cmd/server.
type Matcher interface {
	Submit(pairID string, o *matching.Order) matching.MatchResult
	Cancel(pairID, orderID string) (*matching.Order, error)
	Depth(pairID string, maxLevels int) matching.Depth
}

// PairInfo is the minimal trading-pair data the order domain needs.
// Avoids a direct import of internal/market in the domain layer.
type PairInfo struct {
	PairID       string
	Active       bool
	MinOrderSize float64
	// PriceDecimals is the number of decimal places a price is quoted to.
	// Used to convert float prices into the integer ticks the engine matches on.
	PriceDecimals int
}

// PairService is the narrow interface order needs from the market domain.
// Satisfied by a market-backed adapter wired in cmd/server.
type PairService interface {
	GetPair(ctx context.Context, pairID string) (*PairInfo, error)
}

// ─── Domain Errors ───────────────────────────────────────────────────────────
var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderNotCancellable = errors.New("order cannot be cancelled in current state")
	ErrSelfTrade           = errors.New("self-trade prevention: buyer and seller are the same user")

	ErrInvalidSide       = errors.New("side must be 'buy' or 'sell'")
	ErrInvalidType       = errors.New("type must be 'market', 'limit', 'stop-limit' or 'oco'")
	ErrPriceRequired     = errors.New("price is required and must be greater than zero for limit orders")
	ErrPriceNotAllowed   = errors.New("price must not be set for market orders")
	ErrStopPriceRequired = errors.New("stop_price is required and must be greater than zero for stop-limit/oco orders")
	ErrBelowMinOrderSize = errors.New("quantity is below the minimum order size for this pair")
	ErrPairNotFound      = errors.New("trading pair not found")
	ErrPairSuspended     = errors.New("trading pair is suspended")
)
