package order

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"

	"drexa/internal/auth"
)

type OrderRequest struct {
	PairID string    `json:"pair_id"`
	Side   OrderSide `json:"side"`
	Type   OrderType `json:"type"`

	Quantity float64 `json:"quantity"`

	// Price is the limit price. Required for limit and stop-limit orders, and
	// for the take-profit leg of an OCO order. Omitted for market orders.
	Price *float64 `json:"price,omitempty"`

	// StopPrice is the trigger price. Required for stop-limit and OCO orders.
	StopPrice *float64 `json:"stop_price,omitempty"`
}

// MessageResponse is the standard JSON envelope for handler responses.
type MessageResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func sendJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func HandleOrder(orderSvc Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.JWTClaims)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req OrderRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{
				Error: "invalid input",
			})
			return
		}

		if req.PairID == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{
				Error: "pair_id is required",
			})
			return
		}

		if req.Quantity <= 0 {
			sendJSON(w, http.StatusBadRequest, MessageResponse{
				Error: "quantity must be greater than zero",
			})
			return
		}

		order, err := orderSvc.CreateOrder(
			r.Context(),
			claims.UserID,
			req,
		)
		if err != nil {
			switch {
			case errors.Is(err, ErrPairNotFound):
				sendJSON(w, http.StatusNotFound, MessageResponse{Error: err.Error()})
			case errors.Is(err, ErrInvalidSide),
				errors.Is(err, ErrInvalidType),
				errors.Is(err, ErrPriceRequired),
				errors.Is(err, ErrPriceNotAllowed),
				errors.Is(err, ErrStopPriceRequired),
				errors.Is(err, ErrBelowMinOrderSize),
				errors.Is(err, ErrPairSuspended):
				sendJSON(w, http.StatusBadRequest, MessageResponse{Error: err.Error()})
			default:
				log.Ctx(r.Context()).Error().Err(err).Msg("order: create failed")
				sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "internal server error"})
			}
			return
		}

		sendJSON(w, http.StatusCreated, order)
	})
}

// HandleOrderBook returns a depth snapshot of a pair's resting book.
// Public market data — no auth required.
// Route: GET /api/v1/market/orderbook/{pairID}?depth=50
func HandleOrderBook(orderSvc Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pairID := r.PathValue("pairID")
		if pairID == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "pairID is required"})
			return
		}

		// Default to 50 levels per side; clamp to a sane maximum.
		depth := 50
		if q := r.URL.Query().Get("depth"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 {
				depth = n
			}
		}
		if depth > 500 {
			depth = 500
		}

		ob, err := orderSvc.OrderBookDepth(r.Context(), pairID, depth)
		if err != nil {
			switch {
			case errors.Is(err, ErrPairNotFound):
				sendJSON(w, http.StatusNotFound, MessageResponse{Error: err.Error()})
			default:
				log.Ctx(r.Context()).Error().Err(err).Msg("order: orderbook failed")
				sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "internal server error"})
			}
			return
		}

		sendJSON(w, http.StatusOK, ob)
	})
}

// HandleCancelOrder cancels a resting order owned by the caller.
// Route: DELETE /api/v1/orders/{orderID}
func HandleCancelOrder(orderSvc Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.JWTClaims)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		orderID := r.PathValue("orderID")
		if orderID == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "orderID is required"})
			return
		}

		o, err := orderSvc.CancelOrder(r.Context(), claims.UserID, orderID)
		if err != nil {
			switch {
			case errors.Is(err, ErrOrderNotFound):
				sendJSON(w, http.StatusNotFound, MessageResponse{Error: err.Error()})
			case errors.Is(err, ErrOrderNotCancellable):
				sendJSON(w, http.StatusConflict, MessageResponse{Error: err.Error()})
			default:
				log.Ctx(r.Context()).Error().Err(err).Msg("order: cancel failed")
				sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "internal server error"})
			}
			return
		}

		sendJSON(w, http.StatusOK, o)
	})
}

// HandleListOrders returns the caller's orders, newest first.
// Route: GET /api/v1/orders?status=open|closed|all&pair_id=BTC_USDC&limit=100
// Always responds with a JSON array (never null).
func HandleListOrders(orderSvc Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.JWTClaims)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		f := OrderFilter{
			PairID: r.URL.Query().Get("pair_id"),
			Status: FilterAll,
			Limit:  200,
		}
		switch OrderStatusFilter(r.URL.Query().Get("status")) {
		case FilterOpen:
			f.Status = FilterOpen
		case FilterClosed:
			f.Status = FilterClosed
		}
		if q := r.URL.Query().Get("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 {
				f.Limit = n
			}
		}
		if f.Limit > 500 {
			f.Limit = 500
		}

		orders, err := orderSvc.ListOrders(r.Context(), claims.UserID, f)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("order: list failed")
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "internal server error"})
			return
		}
		if orders == nil {
			orders = []Order{}
		}
		sendJSON(w, http.StatusOK, orders)
	})
}

// HandleListTrades returns the caller's executed fills, newest first.
// Route: GET /api/v1/trades?limit=100
// Always responds with a JSON array (never null).
func HandleListTrades(orderSvc Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.JWTClaims)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		limit := 100
		if q := r.URL.Query().Get("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 500 {
			limit = 500
		}

		trades, err := orderSvc.ListTrades(r.Context(), claims.UserID, limit)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("order: list trades failed")
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "internal server error"})
			return
		}
		if trades == nil {
			trades = []TradeView{}
		}
		sendJSON(w, http.StatusOK, trades)
	})
}
