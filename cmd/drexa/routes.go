package main

import (
	"net/http"
	"time"

	"drexa/internal/auth"
	"drexa/internal/market"
	"drexa/internal/sharedwallet"
	"drexa/internal/wallet"
)

func addRoutes(
	mux *http.ServeMux,
	authUc auth.AuthUsecase,
	kycUc auth.KycUsecase,
	adminKycUc auth.AdminKycUsecase,
	tokenSvc auth.TokenService,
	rateLimiter auth.RateLimiter,
	walletUc wallet.WalletUsecase,
	cryptoWalletUc wallet.CryptoWalletUsecase,
	adminWalletUc wallet.AdminWalletUsecase,
	sharedWalletUc sharedwallet.WalletService,
	sharedTransferUc sharedwallet.InternalTransferService,
	sharedTxRepo sharedwallet.TransactionRepository,
	marketHub *market.Hub,
	secureCookies bool,
) {
	mux.Handle("/", http.NotFoundHandler())

	jwt := auth.JWTMiddleware(tokenSvc)

	// Brute-force protection on credential endpoints, keyed per client IP.
	loginRL := auth.RateLimitMiddleware(rateLimiter, "login", 10, 15*time.Minute)
	registerRL := auth.RateLimitMiddleware(rateLimiter, "register", 5, time.Hour)

	// ── Public auth ──────────────────────────────────────────────────────────
	mux.Handle("POST /api/v1/auth/register", registerRL(auth.HandleRegister(authUc, secureCookies)))
	mux.Handle("POST /api/v1/auth/login", loginRL(auth.HandleLogin(authUc, secureCookies)))
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

	// ── Wallet — crypto (Tatum) on-chain deposit addresses + balances (JWT required) ──
	mux.Handle("GET /api/v1/wallet/crypto/assets", jwt(wallet.HandleGetCryptoAssets(cryptoWalletUc)))
	mux.Handle("GET /api/v1/wallet/crypto/address/{currency}", jwt(wallet.HandleGetCryptoAddress(cryptoWalletUc)))

	// ── Payments — Stripe PaymentIntent for embedded deposit form (JWT required) ──
	mux.Handle("POST /api/v1/payments/deposit/intent", jwt(wallet.HandleCreateDepositIntent(walletUc)))

	// ── Wallet — payment provider webhooks (no JWT — secured by signature) ───
	mux.Handle("POST /api/v1/webhooks/deposit", wallet.HandleDepositWebhook(walletUc))

	// ── Admin Wallet (JWT required) ──────────────────────────────────────────
	mux.Handle("GET /api/v1/admin/wallet/withdrawals", jwt(wallet.HandleAdminListWithdrawals(adminWalletUc)))
	mux.Handle("POST /api/v1/admin/wallet/withdrawals/{withdrawal_id}/approve", jwt(wallet.HandleAdminApproveWithdrawal(adminWalletUc)))
	mux.Handle("POST /api/v1/admin/wallet/withdrawals/{withdrawal_id}/reject", jwt(wallet.HandleAdminRejectWithdrawal(adminWalletUc)))

	// ── Shared Wallet — user facing (JWT required) ───────────────────────────
	mux.Handle("POST /api/v1/sharedwallet/create", jwt(sharedwallet.HandleCreateWallet(sharedWalletUc)))
	mux.Handle("GET /api/v1/sharedwallet/balance", jwt(sharedwallet.HandleGetBalance(sharedWalletUc)))
	mux.Handle("POST /api/v1/sharedwallet/withdraw", jwt(sharedwallet.HandleWithdraw(sharedWalletUc)))
	mux.Handle("POST /api/v1/sharedwallet/transfer", jwt(sharedwallet.HandleTransfer(sharedTransferUc)))
	mux.Handle("GET /api/v1/sharedwallet/transactions", jwt(sharedwallet.HandleGetTransactions(sharedTxRepo)))

	// ── Shared Wallet — payment provider webhooks ────────────────────────────
	mux.Handle("POST /api/v1/webhooks/tatum/deposit", sharedwallet.HandleTatumDepositWebhook(sharedWalletUc))

	// ── Market — user facing (WebSocket) ─────────────────────────────────────
	mux.Handle("GET /api/v1/market/stream", market.HandleWebSocket(marketHub))
}
