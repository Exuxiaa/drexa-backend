package order

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rs/zerolog/log"

	"drexa/internal/auth"
)

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
			case errors.Is(err, ErrInvalidSide),
				errors.Is(err, ErrInvalidType),
				errors.Is(err, ErrPriceRequired),
				errors.Is(err, ErrPriceNotAllowed),
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
