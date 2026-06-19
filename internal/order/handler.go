package order

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"

	"drexa/internal/auth"
)

// HandleListOrders returns the authenticated user's orders with optional
// status and pair_id filters.
// Route: GET /api/v1/orders?status=open&pair_id=BTC_USDC&limit=50&offset=0
func HandleListOrders(orderSvc Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.JWTClaims)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		status := r.URL.Query().Get("status")
		pairID := r.URL.Query().Get("pair_id")
		limit, offset := parsePagination(r, 50, 200)

		orders, total, err := orderSvc.ListOrders(r.Context(), claims.UserID, status, pairID, limit, offset)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("order: list failed")
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "internal server error"})
			return
		}

		sendJSON(w, http.StatusOK, map[string]any{
			"orders": orders,
			"total":  total,
		})
	})
}

// HandleListTrades returns the authenticated user's trade history with side
// and role derived from their perspective.
// Route: GET /api/v1/trades?pair_id=BTC_USDC&limit=50&offset=0
func HandleListTrades(orderSvc Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(auth.UserClaimsKey).(*auth.JWTClaims)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		pairID := r.URL.Query().Get("pair_id")
		limit, offset := parsePagination(r, 50, 200)

		trades, total, err := orderSvc.ListTrades(r.Context(), claims.UserID, pairID, limit, offset)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("order: list trades failed")
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "internal server error"})
			return
		}

		sendJSON(w, http.StatusOK, map[string]any{
			"trades": trades,
			"total":  total,
		})
	})
}

// parsePagination extracts limit and offset from query params with defaults.
func parsePagination(r *http.Request, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if q := r.URL.Query().Get("offset"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n >= 0 {
			offset = n
		}
	}
	return
}

type OrderRequest struct {
	PairID string    `json:"pair_id"`
	Side   OrderSide `json:"side"`
	Type   OrderType `json:"type"`

	Quantity float64 `json:"quantity"`

	// LIMIT only
	Price *float64 `json:"price,omitempty"`
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
			case errors.Is(err, ErrInsufficientBalance):
				sendJSON(w, http.StatusUnprocessableEntity, MessageResponse{Error: err.Error()})
			case errors.Is(err, ErrInvalidSide),
				errors.Is(err, ErrInvalidType),
				errors.Is(err, ErrPriceRequired),
				errors.Is(err, ErrPriceNotAllowed),
				errors.Is(err, ErrBelowMinOrderSize),
				errors.Is(err, ErrFiatNotTradable),
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
