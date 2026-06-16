package main

import (
	"net/http"

	"drexa/internal/auth"
	"drexa/internal/market"
	"drexa/internal/wallet"
)

func addRoutes(
	mux *http.ServeMux,
	authUc auth.AuthUsecase,
	kycUc auth.KycUsecase,
	adminKycUc auth.AdminKycUsecase,
	tokenSvc auth.TokenService,
	fbVerifier auth.FirebaseVerifier,
	walletUc wallet.WalletUsecase,
	adminWalletUc wallet.AdminWalletUsecase,
	marketHub *market.Hub,
	secureCookies bool,
) {
	mux.Handle("/", http.NotFoundHandler())

	jwt := auth.JWTMiddleware(tokenSvc)

	// ── Public auth ──────────────────────────────────────────────────────────
	mux.Handle("POST /api/v1/auth/signin", auth.HandleFirebaseSignIn(authUc, fbVerifier, secureCookies))
	mux.Handle("POST /api/v1/auth/logout", auth.HandleLogout(authUc))
	mux.Handle("POST /api/v1/auth/refresh", auth.HandleRefreshToken(authUc, secureCookies))

	// ── Protected auth (JWT required) ────────────────────────────────────────
	mux.Handle("POST /api/v1/auth/logout/all", jwt(auth.HandleLogoutAll(authUc)))
	mux.Handle("POST /api/v1/auth/pin/set", jwt(auth.HandleSetTradingPin(authUc)))
	mux.Handle("POST /api/v1/auth/pin/verify", jwt(auth.HandleVerifyTradingPin(authUc)))
	mux.Handle("POST /api/v1/auth/verify/phone", jwt(auth.HandleVerifyPhone(authUc)))

	// ── KYC — user facing (JWT required) ─────────────────────────────────────
	_ = kycUc // TODO: implement KYC handlers

	// ── KYC — admin facing (JWT required) ────────────────────────────────────
	_ = adminKycUc // TODO: implement admin KYC handlers

	// ── Wallet — user facing (JWT required) ──────────────────────────────────
	mux.Handle("GET /api/v1/wallet/balances", jwt(wallet.HandleGetBalances(walletUc)))
	mux.Handle("GET /api/v1/wallet/balance/{currency}", jwt(wallet.HandleGetBalance(walletUc)))
	mux.Handle("POST /api/v1/wallet/deposit", jwt(wallet.HandleInitiateDeposit(walletUc)))
	mux.Handle("POST /api/v1/wallet/withdraw", jwt(wallet.HandleInitiateWithdrawal(walletUc)))
	mux.Handle("GET /api/v1/wallet/transactions", jwt(wallet.HandleGetTransactions(walletUc)))

	// ── Wallet — payment provider webhooks (no JWT — secured by signature) ───
	mux.Handle("POST /api/v1/webhooks/deposit", wallet.HandleDepositWebhook(walletUc))

	// ── Admin Wallet (JWT required) ──────────────────────────────────────────
	mux.Handle("GET /api/v1/admin/wallet/withdrawals", jwt(wallet.HandleAdminListWithdrawals(adminWalletUc)))
	mux.Handle("POST /api/v1/admin/wallet/withdrawals/{withdrawal_id}/approve", jwt(wallet.HandleAdminApproveWithdrawal(adminWalletUc)))
	mux.Handle("POST /api/v1/admin/wallet/withdrawals/{withdrawal_id}/reject", jwt(wallet.HandleAdminRejectWithdrawal(adminWalletUc)))

	// ── Market — user facing (WebSocket) ─────────────────────────────────────
	mux.Handle("GET /api/v1/market/stream", market.HandleWebSocket(marketHub))
}
