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
	TypeMarket OrderType = "market"
	TypeLimit  OrderType = "limit"
)

type OrderStatus string

const (
	StatusPending         OrderStatus = "pending"
	StatusOpen            OrderStatus = "open"
	StatusPartiallyFilled OrderStatus = "partially_filled"
	StatusFilled          OrderStatus = "filled"
	StatusCancelled       OrderStatus = "cancelled"
)

// ─── Entities ────────────────────────────────────────────────────────────────

// Order is a user's intent to buy or sell a trading pair.
type Order struct {
	OrderID        string      `gorm:"primaryKey;column:order_id"`
	UserID         string      `gorm:"column:user_id;index"`
	PairID         string      `gorm:"column:pair_id;index"`
	Side           OrderSide   `gorm:"column:side"`
	Type           OrderType   `gorm:"column:type"`
	Status         OrderStatus `gorm:"column:status;default:pending"`
	Price          *float64    `gorm:"column:price;type:numeric(36,18)"` // nil for market orders
	Quantity       float64     `gorm:"column:quantity;type:numeric(36,18)"`
	FilledQuantity float64     `gorm:"column:filled_quantity;type:numeric(36,18);default:0"`
	LockedAmount   float64     `gorm:"column:locked_amount;type:numeric(36,18);default:0"`
	Fee            float64     `gorm:"column:fee;type:numeric(36,18);default:0"`
	CreatedAt      time.Time   `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time   `gorm:"column:updated_at;autoUpdateTime"`
}

// Trade is the immutable record produced when two orders match.
type Trade struct {
	TradeID      string    `gorm:"primaryKey;column:trade_id"`
	PairID       string    `gorm:"column:pair_id;index"`
	MakerOrderID string    `gorm:"column:maker_order_id"`
	TakerOrderID string    `gorm:"column:taker_order_id"`
	Price        float64   `gorm:"column:price;type:numeric(36,18)"`
	Quantity     float64   `gorm:"column:quantity;type:numeric(36,18)"`
	MakerFee     float64   `gorm:"column:maker_fee;type:numeric(36,18);default:0"`
	TakerFee     float64   `gorm:"column:taker_fee;type:numeric(36,18);default:0"`
	ExecutedAt   time.Time `gorm:"column:executed_at;autoCreateTime"`
}

// ─── Service & Repository Interfaces ─────────────────────────────────────────

// Service is the order domain's business-logic entrypoint.
type Service interface {
	CreateOrder(ctx context.Context, userID string, req OrderRequest) (*Order, error)
	CancelOrder(ctx context.Context, userID, orderID string) (*Order, error)
}

// Repository persists orders and trades.
type Repository interface {
	Create(ctx context.Context, o *Order) error
	FindByID(ctx context.Context, orderID string) (*Order, error)
	FindByUserID(ctx context.Context, userID string) ([]Order, error)
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
	ErrInvalidType       = errors.New("type must be 'market' or 'limit'")
	ErrPriceRequired     = errors.New("price is required and must be greater than zero for limit orders")
	ErrPriceNotAllowed   = errors.New("price must not be set for market orders")
	ErrBelowMinOrderSize = errors.New("quantity is below the minimum order size for this pair")
	ErrPairNotFound      = errors.New("trading pair not found")
	ErrPairSuspended     = errors.New("trading pair is suspended")
)
