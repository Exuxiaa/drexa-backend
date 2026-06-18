package main

import (
	"net/http"

	"drexa/internal/auth"
	"drexa/internal/kyc"
	"drexa/internal/market"
	"drexa/internal/order"
	"drexa/internal/wallet"
)

func addRoutes(
	mux *http.ServeMux,
	authUc auth.AuthUsecase,
	kycH *kyc.Handler,
	orderSvc order.Service,
	walletUc wallet.WalletUsecase,
	adminWalletUc wallet.AdminWalletUsecase,
	marketHub *market.Hub,
	tokenSvc auth.TokenService,
) {
	mux.Handle("/", http.NotFoundHandler())

	jwt   := auth.JWTMiddleware(tokenSvc)
	admin := auth.RequireRole(auth.RoleAdmin)

	// ── Public auth ───────────────────────────────────────────────────────────
	mux.Handle("POST /api/v1/auth/register", auth.HandleRegister(authUc))
	mux.Handle("POST /api/v1/auth/login",    auth.HandleLogin(authUc))
	mux.Handle("POST /api/v1/auth/logout",   auth.HandleLogout(authUc))
	mux.Handle("POST /api/v1/auth/refresh",  auth.HandleRefreshToken(authUc))

	// ── Protected auth (JWT required) ─────────────────────────────────────────
	mux.Handle("POST /api/v1/auth/logout/all",       jwt(auth.HandleLogoutAll(authUc)))
	mux.Handle("POST /api/v1/auth/password/change",  jwt(auth.HandleChangePassword(authUc)))
	mux.Handle("POST /api/v1/auth/otp/phone/send",   jwt(auth.HandleSendPhoneOTP(authUc)))
	mux.Handle("POST /api/v1/auth/otp/phone/verify", jwt(auth.HandleVerifyPhoneOTP(authUc)))
	mux.Handle("POST /api/v1/auth/pin/set",          jwt(auth.HandleSetTradingPIN(authUc)))
	mux.Handle("POST /api/v1/auth/pin/verify",       jwt(auth.HandleVerifyTradingPIN(authUc)))

	// ── 2FA (TOTP) ────────────────────────────────────────────────────────────
	mux.Handle("POST /api/v1/auth/login/2fa",  auth.HandleLoginTwoFA(authUc, tokenSvc))
	mux.Handle("POST /api/v1/auth/2fa/setup",  jwt(auth.HandleTwoFASetup(authUc)))
	mux.Handle("POST /api/v1/auth/2fa/enable", jwt(auth.HandleTwoFAEnable(authUc)))
	mux.Handle("POST /api/v1/auth/2fa/disable",jwt(auth.HandleTwoFADisable(authUc)))

	// ── KYC — user facing (JWT required) ──────────────────────────────────────
	mux.Handle("POST /api/v1/kyc/submit", jwt(http.HandlerFunc(kycH.HandleSubmit)))
	mux.Handle("GET /api/v1/kyc/status",  jwt(http.HandlerFunc(kycH.HandleGetStatus)))

	// ── KYC — admin facing (JWT + admin role) ─────────────────────────────────
	mux.Handle("GET /api/v1/admin/kyc",                    jwt(admin(http.HandlerFunc(kycH.HandleAdminList))))
	mux.Handle("GET /api/v1/admin/kyc/{id}",               jwt(admin(http.HandlerFunc(kycH.HandleAdminGet))))
	mux.Handle("POST /api/v1/admin/kyc/{id}/approve",      jwt(admin(http.HandlerFunc(kycH.HandleAdminApprove))))
	mux.Handle("POST /api/v1/admin/kyc/{id}/reject",       jwt(admin(http.HandlerFunc(kycH.HandleAdminReject))))

	// ── Orders (JWT required) ─────────────────────────────────────────────────
	mux.Handle("POST /api/v1/orders",               jwt(order.HandleOrder(orderSvc)))
	mux.Handle("DELETE /api/v1/orders/{orderID}",   jwt(order.HandleCancelOrder(orderSvc)))

	// ── Wallet — user facing (JWT required) ───────────────────────────────────
	mux.Handle("GET /api/v1/wallet/balances",            jwt(wallet.HandleGetBalances(walletUc)))
	mux.Handle("GET /api/v1/wallet/balances/{currency}", jwt(wallet.HandleGetBalance(walletUc)))
	mux.Handle("POST /api/v1/wallet/deposit",            jwt(wallet.HandleInitiateDeposit(walletUc)))
	mux.Handle("POST /api/v1/wallet/withdraw",           jwt(wallet.HandleInitiateWithdrawal(walletUc)))
	mux.Handle("GET /api/v1/wallet/transactions",        jwt(wallet.HandleGetTransactions(walletUc)))

	// ── Wallet — payment provider webhook (public; verify signature in prod) ───
	mux.Handle("POST /api/v1/wallet/deposit/webhook", wallet.HandleDepositWebhook(walletUc))

	// ── Wallet — admin facing (JWT + admin role) ──────────────────────────────
	mux.Handle("GET /api/v1/admin/wallet/withdrawals",                       jwt(admin(wallet.HandleAdminListWithdrawals(adminWalletUc))))
	mux.Handle("POST /api/v1/admin/wallet/withdrawals/{withdrawal_id}/approve", jwt(admin(wallet.HandleAdminApproveWithdrawal(adminWalletUc))))
	mux.Handle("POST /api/v1/admin/wallet/withdrawals/{withdrawal_id}/reject",  jwt(admin(wallet.HandleAdminRejectWithdrawal(adminWalletUc))))

	// ── Market data (public real-time WebSocket feed) ─────────────────────────
	mux.Handle("/api/v1/market/ws", market.HandleWebSocket(marketHub))
}
