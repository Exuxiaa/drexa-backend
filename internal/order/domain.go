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
//
// json tags are required: this struct is returned directly by POST /orders,
// DELETE /orders/{id} and (in a list) GET /orders. Without them Go marshals the
// Go field names (PascalCase) and the frontend — which reads snake_case, matching
// the rest of the API — sees every field as undefined.
type Order struct {
	OrderID        string      `gorm:"primaryKey;column:order_id" json:"order_id"`
	UserID         string      `gorm:"column:user_id;index" json:"user_id"`
	PairID         string      `gorm:"column:pair_id;index" json:"pair_id"`
	Side           OrderSide   `gorm:"column:side" json:"side"`
	Type           OrderType   `gorm:"column:type" json:"type"`
	Status         OrderStatus `gorm:"column:status;default:pending" json:"status"`
	Price          *float64    `gorm:"column:price;type:numeric(36,18)" json:"price"` // nil for market orders
	Quantity       float64     `gorm:"column:quantity;type:numeric(36,18)" json:"quantity"`
	FilledQuantity float64     `gorm:"column:filled_quantity;type:numeric(36,18);default:0" json:"filled_quantity"`
	LockedAmount   float64     `gorm:"column:locked_amount;type:numeric(36,18);default:0" json:"locked_amount"`
	Fee            float64     `gorm:"column:fee;type:numeric(36,18);default:0" json:"fee"`
	CreatedAt      time.Time   `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time   `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
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

// UserTrade is a Trade enriched with the caller's perspective: which side they
// were on and whether they were the maker or taker. Returned by ListTrades.
type UserTrade struct {
	TradeID    string    `json:"trade_id"`
	PairID     string    `json:"pair_id"`
	Side       OrderSide `json:"side"`
	Price      float64   `json:"price"`
	Quantity   float64   `json:"quantity"`
	Fee        float64   `json:"fee"` // caller's fee (maker or taker)
	Role       string    `json:"role"` // "maker" | "taker"
	ExecutedAt time.Time `json:"executed_at"`
}

// ─── Service & Repository Interfaces ─────────────────────────────────────────

// Service is the order domain's business-logic entrypoint.
type Service interface {
	CreateOrder(ctx context.Context, userID string, req OrderRequest) (*Order, error)
	CancelOrder(ctx context.Context, userID, orderID string) (*Order, error)
	CleanupOpenOrders(ctx context.Context) error
	// OrderBookDepth returns the live aggregated book for a pair in real
	// (float) prices, best prices first. maxLevels <= 0 returns every level.
	OrderBookDepth(ctx context.Context, pairID string, maxLevels int) (*OrderBookSnapshot, error)
	// ListOrders returns a filtered, paginated list of the user's orders.
	// status and pairID are optional filters (empty string = no filter).
	ListOrders(ctx context.Context, userID, status, pairID string, limit, offset int) ([]Order, int64, error)
	// ListTrades returns the user's trade history with side/role derived from
	// their perspective. pairID is an optional filter.
	ListTrades(ctx context.Context, userID, pairID string, limit, offset int) ([]UserTrade, int64, error)
}

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
	FindAllOpen(ctx context.Context) ([]Order, error)
	// FindByUserIDFiltered returns paginated orders for a user with optional
	// status and pairID filters. Empty string = no filter.
	FindByUserIDFiltered(ctx context.Context, userID, status, pairID string, limit, offset int) ([]Order, int64, error)
	// FindTradesByUserID returns paginated trades for a user enriched with
	// their side and role. pairID is an optional filter.
	FindTradesByUserID(ctx context.Context, userID, pairID string, limit, offset int) ([]UserTrade, int64, error)
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
	PairID        string
	BaseCoin      string // e.g. "BTC"
	QuoteCoin     string // e.g. "USD"
	Active        bool
	MinOrderSize  float64
	// PriceDecimals is the number of decimal places a price is quoted to.
	// Used to convert float prices into the integer ticks the engine matches on.
	PriceDecimals int
}

// PairService is the narrow interface order needs from the market domain.
// Satisfied by a market-backed adapter wired in cmd/server.
type PairService interface {
	GetPair(ctx context.Context, pairID string) (*PairInfo, error)
}

// WalletService is the narrow interface the order domain needs from the wallet
// domain to validate available balance before persisting an order, lock funds,
// and settle trades.
type WalletService interface {
	AvailableBalance(ctx context.Context, userID, currency string) (int64, error)
	LockBalance(ctx context.Context, userID, currency string, amount int64) error
	UnlockBalance(ctx context.Context, userID, currency string, amount int64) error
	SettleTrade(ctx context.Context, tradeID string,
		makerID, makerSpentCurrency string, makerSpentAmount int64, makerReceivedCurrency string, makerReceivedAmount int64,
		takerID, takerSpentCurrency string, takerSpentAmount int64, takerReceivedCurrency string, takerReceivedAmount int64, takerUnlock bool) error
}

// ─── Domain Errors ───────────────────────────────────────────────────────────
var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderNotCancellable = errors.New("order cannot be cancelled in current state")
	ErrSelfTrade           = errors.New("self-trade prevention: buyer and seller are the same user")

	ErrInvalidSide         = errors.New("side must be 'buy' or 'sell'")
	ErrInvalidType         = errors.New("type must be 'market' or 'limit'")
	ErrPriceRequired       = errors.New("price is required and must be greater than zero for limit orders")
	ErrPriceNotAllowed     = errors.New("price must not be set for market orders")
	ErrBelowMinOrderSize   = errors.New("quantity is below the minimum order size for this pair")
	ErrPairNotFound        = errors.New("trading pair not found")
	ErrPairSuspended       = errors.New("trading pair is suspended")
	ErrFiatNotTradable     = errors.New("fiat currency is not tradable; deposit or withdraw USD via the wallet instead")
	ErrInsufficientBalance = errors.New("insufficient balance to place this order")
)
