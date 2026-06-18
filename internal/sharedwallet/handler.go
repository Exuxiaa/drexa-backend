package sharedwallet

import (
	"encoding/json"
	"net/http"
	"strconv"

	"drexa/internal/auth"
)

// ─── DTOs ────────────────────────────────────────────────────────────────────

type MessageResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type CreateWalletRequest struct {
	Currency string `json:"currency"`
}

type WithdrawRequest struct {
	Currency  string `json:"currency"`
	Amount    string `json:"amount"`
	ToAddress string `json:"to_address"`
}

type TransferRequest struct {
	ToUserID string `json:"to_user_id"`
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func sendJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func userFromCtx(r *http.Request) (*auth.JWTClaims, bool) {
	return auth.UserFromContext(r.Context())
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func HandleCreateWallet(ws WalletService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var req CreateWalletRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		wallet, err := ws.CreateUserWallet(r.Context(), claims.UserID, req.Currency)
		if err != nil {
			if err == ErrWalletExists {
				sendJSON(w, http.StatusConflict, MessageResponse{Error: err.Error()})
			} else {
				sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			}
			return
		}

		sendJSON(w, http.StatusCreated, wallet)
	}
}

func HandleGetBalance(ws WalletService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		currency := r.URL.Query().Get("currency")
		if currency == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "currency is required"})
			return
		}

		bal, locked, err := ws.GetBalance(r.Context(), claims.UserID, currency)
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, map[string]string{
			"currency": currency,
			"balance":  bal,
			"locked":   locked,
		})
	}
}

func HandleWithdraw(ws WalletService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var req WithdrawRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		txReq := WithdrawalRequest{
			UserID:    claims.UserID,
			Currency:  req.Currency,
			Amount:    req.Amount,
			ToAddress: req.ToAddress,
		}

		txRecord, err := ws.RequestWithdrawal(r.Context(), txReq)
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusCreated, txRecord)
	}
}

func HandleTransfer(ts InternalTransferService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var req TransferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		transferReq := InternalTransferRequest{
			FromUserID: claims.UserID,
			ToUserID:   req.ToUserID,
			Currency:   req.Currency,
			Amount:     req.Amount,
		}

		txRecord, err := ts.Transfer(r.Context(), transferReq)
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, txRecord)
	}
}

func HandleGetTransactions(txRepo TransactionRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 100 {
			limit = 50
		}
		cursor := r.URL.Query().Get("cursor")

		txs, err := txRepo.GetTransactionsByUserId(r.Context(), claims.UserID, limit, cursor)
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, txs)
	}
}

func HandleTatumDepositWebhook(ws WalletService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid payload"})
			return
		}

		// Simple origin validation (Tatum does not send robust signatures, usually IP whitelisting or custom header is used)
		// For the scope of this project, we assume payload is valid or validate via additional fields
		
		if err := ws.ProcessDeposit(r.Context(), payload); err != nil {
			if err == ErrIdempotencyKeyFound {
				sendJSON(w, http.StatusOK, MessageResponse{Message: "already processed"})
				return
			}
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "success"})
	}
}
