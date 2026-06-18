package wallet

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"drexa/internal/auth"
)

// ─── DTOs ────────────────────────────────────────────────────────────────────

type BalanceResponse struct {
	WalletID  string `json:"wallet_id"`
	Currency  string `json:"currency"`
	Balance   int64  `json:"balance"`    // total balance in smallest unit
	Locked    int64  `json:"locked"`     // amount reserved (open orders / pending withdrawal)
	Available int64  `json:"available"`  // spendable = balance - locked
	Status    string `json:"status"`
}

type InitiateDepositHTTPRequest struct {
	Amount   int64  `json:"amount"`   // in smallest unit (e.g. 1 IDR = 1)
	Currency string `json:"currency"` // e.g. "IDR"
}

type InitiateDepositResponse struct {
	DepositID   string `json:"deposit_id"`
	ProviderRef string `json:"provider_ref"`
	ExpiresAt   string `json:"expires_at"`
	Message     string `json:"message"`
}

type DepositIntentHTTPRequest struct {
	Amount   int64  `json:"amount"`   // in smallest unit (cents); e.g. $5.00 = 500
	Currency string `json:"currency"` // optional — defaults to USD
}

type DepositIntentResponse struct {
	ClientSecret string `json:"client_secret"` // handed to Stripe Elements on the frontend
	TxID         string `json:"tx_id"`         // deposit record id, for client-side reference
}

type InitiateWithdrawalHTTPRequest struct {
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	BankCode      string `json:"bank_code"`      // e.g. "BCA"
	AccountNumber string `json:"account_number"` // destination bank account
	AccountName   string `json:"account_name"`
}

type TransactionResponse struct {
	TxID          string `json:"tx_id"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Amount        int64  `json:"amount"`
	BalanceBefore int64  `json:"balance_before"`
	BalanceAfter  int64  `json:"balance_after"`
	Currency      string `json:"currency"`
	Description   string `json:"description"`
	CreatedAt     string `json:"created_at"`
}

type ConfirmDepositWebhookRequest struct {
	ProviderRef string `json:"provider_ref"`
}

type MessageResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
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

// normalizeCurrency upper-cases and trims a currency code from client input so that, e.g.,
// "usd" from the URL path matches the canonical "USD" stored on the wallet.
func normalizeCurrency(s string) CurrencyCode {
	return CurrencyCode(strings.ToUpper(strings.TrimSpace(s)))
}

// ─── User-Facing Wallet Handlers ─────────────────────────────────────────────

// HandleGetBalances returns all currency wallets for the authenticated user
func HandleGetBalances(uc WalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		wallets, err := uc.GetAllBalances(r.Context(), claims.UserID)
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		resp := make([]BalanceResponse, 0, len(wallets))
		for _, wlt := range wallets {
			resp = append(resp, BalanceResponse{
				WalletID:  wlt.WalletID,
				Currency:  string(wlt.Currency),
				Balance:   wlt.Balance,
				Locked:    wlt.Locked,
				Available: wlt.Available(),
				Status:    string(wlt.Status),
			})
		}

		sendJSON(w, http.StatusOK, resp)
	}
}

// HandleGetBalance returns a single currency wallet for the authenticated user
func HandleGetBalance(uc WalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		currency := normalizeCurrency(r.PathValue("currency"))
		if currency == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "currency is required"})
			return
		}

		wlt, err := uc.GetBalance(r.Context(), claims.UserID, currency)
		if err == ErrWalletNotFound {
			// A user who hasn't transacted in this currency yet simply has a zero balance —
			// report that rather than a 404 so the wallet UI can render $0.00 cleanly.
			sendJSON(w, http.StatusOK, BalanceResponse{
				Currency: string(currency),
				Status:   string(WalletStatusActive),
			})
			return
		}
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, BalanceResponse{
			WalletID:  wlt.WalletID,
			Currency:  string(wlt.Currency),
			Balance:   wlt.Balance,
			Locked:    wlt.Locked,
			Available: wlt.Available(),
			Status:    string(wlt.Status),
		})
	}
}

// HandleInitiateDeposit creates a payment session and returns the provider URL
func HandleInitiateDeposit(uc WalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var req InitiateDepositHTTPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}
		if req.Amount <= 0 || req.Currency == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "amount and currency are required"})
			return
		}

		depositReq, err := uc.InitiateDeposit(r.Context(), claims.UserID, &InitiateDepositRequest{
			Amount:    req.Amount,
			Currency:  normalizeCurrency(req.Currency),
			UserEmail: claims.Email,
		})
		if err != nil {
			switch err {
			case ErrInvalidAmount:
				sendJSON(w, http.StatusBadRequest, MessageResponse{Error: err.Error()})
			case ErrWalletSuspended, ErrWalletClosed:
				sendJSON(w, http.StatusForbidden, MessageResponse{Error: err.Error()})
			default:
				sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "deposit initiation failed"})
			}
			return
		}

		sendJSON(w, http.StatusCreated, InitiateDepositResponse{
			DepositID:   depositReq.DepositID,
			ProviderRef: depositReq.ProviderRef,
			ExpiresAt:   depositReq.ExpiresAt.Format("2006-01-02T15:04:05Z"),
			Message:     "deposit session created",
		})
	}
}

// HandleCreateDepositIntent creates a Stripe PaymentIntent and returns its client secret.
// The frontend's embedded Stripe Elements form completes the payment; the wallet is credited
// later by the deposit webhook.
func HandleCreateDepositIntent(uc WalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var req DepositIntentHTTPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}
		if req.Amount <= 0 {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "amount is required"})
			return
		}

		currency := normalizeCurrency(req.Currency)
		if currency == "" {
			currency = CurrencyUSD
		}

		intent, err := uc.CreateDepositIntent(r.Context(), claims.UserID, &InitiateDepositRequest{
			Amount:    req.Amount,
			Currency:  currency,
			UserEmail: claims.Email,
		})
		if err != nil {
			switch err {
			case ErrInvalidAmount:
				sendJSON(w, http.StatusBadRequest, MessageResponse{Error: err.Error()})
			case ErrWalletSuspended, ErrWalletClosed:
				sendJSON(w, http.StatusForbidden, MessageResponse{Error: err.Error()})
			default:
				sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "deposit intent failed"})
			}
			return
		}

		sendJSON(w, http.StatusCreated, DepositIntentResponse{
			ClientSecret: intent.ClientSecret,
			TxID:         intent.DepositID,
		})
	}
}

// HandleDepositWebhook is called by the payment provider on successful payment.
// Secure this endpoint with provider signature verification in production.
func HandleDepositWebhook(uc WalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: verify Stripe/Midtrans webhook signature here before processing
		var req ConfirmDepositWebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid webhook payload"})
			return
		}

		if err := uc.ConfirmDeposit(r.Context(), req.ProviderRef); err != nil {
			switch err {
			case ErrDepositNotFound:
				sendJSON(w, http.StatusNotFound, MessageResponse{Error: err.Error()})
			case ErrDepositAlreadyDone, ErrDepositExpired:
				sendJSON(w, http.StatusConflict, MessageResponse{Error: err.Error()})
			default:
				sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "confirm deposit failed"})
			}
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "deposit confirmed"})
	}
}

// HandleInitiateWithdrawal queues a withdrawal request (pending admin approval)
func HandleInitiateWithdrawal(uc WalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		// KYC gate — withdrawal requires an approved KYC submission (level >= 1)
		if claims.KycLevel < 1 {
			sendJSON(w, http.StatusForbidden, MessageResponse{Error: "KYC verification required before withdrawal"})
			return
		}

		var req InitiateWithdrawalHTTPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		withdrawalReq, err := uc.InitiateWithdrawal(r.Context(), claims.UserID, &InitiateWithdrawalRequest{
			Amount:        req.Amount,
			Currency:      normalizeCurrency(req.Currency),
			BankCode:      req.BankCode,
			AccountNumber: req.AccountNumber,
			AccountName:   req.AccountName,
		})
		if err != nil {
			switch err {
			case ErrInvalidAmount, ErrWithdrawalAmountMin:
				sendJSON(w, http.StatusBadRequest, MessageResponse{Error: err.Error()})
			case ErrInsufficientBalance:
				sendJSON(w, http.StatusUnprocessableEntity, MessageResponse{Error: err.Error()})
			case ErrWalletSuspended, ErrWithdrawalPending:
				sendJSON(w, http.StatusForbidden, MessageResponse{Error: err.Error()})
			default:
				sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "withdrawal initiation failed"})
			}
			return
		}

		sendJSON(w, http.StatusCreated, map[string]any{
			"withdrawal_id": withdrawalReq.WithdrawalID,
			"status":        withdrawalReq.Status,
			"message":       "withdrawal request submitted, pending review",
		})
	}
}

// HandleGetTransactions returns paginated transaction history for the authenticated user
func HandleGetTransactions(uc WalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

		txs, err := uc.GetTransactions(r.Context(), claims.UserID, page, pageSize)
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		resp := make([]TransactionResponse, 0, len(txs))
		for _, tx := range txs {
			resp = append(resp, TransactionResponse{
				TxID:          tx.TxID,
				Type:          string(tx.Type),
				Status:        string(tx.Status),
				Amount:        tx.Amount,
				BalanceBefore: tx.BalanceBefore,
				BalanceAfter:  tx.BalanceAfter,
				Currency:      string(tx.Currency),
				Description:   tx.Description,
				CreatedAt:     tx.CreatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}

		sendJSON(w, http.StatusOK, resp)
	}
}

// ─── Admin Wallet Handlers ────────────────────────────────────────────────────

// HandleAdminListWithdrawals lists all pending withdrawals for admin review
func HandleAdminListWithdrawals(uc AdminWalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withdrawals, err := uc.ListPendingWithdrawals(r.Context())
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}
		sendJSON(w, http.StatusOK, withdrawals)
	}
}

// HandleAdminApproveWithdrawal approves and disburses a pending withdrawal
func HandleAdminApproveWithdrawal(uc AdminWalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		withdrawalID := r.PathValue("withdrawal_id")
		if withdrawalID == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "withdrawal_id is required"})
			return
		}

		if err := uc.ApproveWithdrawal(r.Context(), withdrawalID, claims.UserID); err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "withdrawal approved and disbursed"})
	}
}

// HandleAdminRejectWithdrawal rejects a pending withdrawal and refunds the locked amount
func HandleAdminRejectWithdrawal(uc AdminWalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		withdrawalID := r.PathValue("withdrawal_id")
		if withdrawalID == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "withdrawal_id is required"})
			return
		}

		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		if err := uc.RejectWithdrawal(r.Context(), withdrawalID, claims.UserID, body.Reason); err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "withdrawal rejected"})
	}
}
