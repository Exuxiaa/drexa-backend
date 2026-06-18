package main

import (
	"net/http"

	"drexa/internal/auth"
	"drexa/internal/checkout"
	"drexa/internal/kyc"
	"drexa/internal/market"
	"drexa/internal/order"
	"drexa/internal/p2p"
	"drexa/internal/wallet"
	"drexa/pkg/config"
)

func addRoutes(
	mux *http.ServeMux,
	cfg *config.Config,
	authUc auth.AuthUsecase,
	kycH *kyc.Handler,
	orderSvc order.Service,
	walletUc wallet.WalletUsecase,
	adminWalletUc wallet.AdminWalletUsecase,
	cryptoWalletUc wallet.CryptoWalletUsecase,
	marketHub *market.Hub,
	binance *market.BinanceClient,
	tickerCache *market.TickerCache,
	tokenSvc auth.TokenService,
	checkoutH *checkout.Handler,
	p2pH *p2p.Handler,
) {
	mux.Handle("/", http.NotFoundHandler())

	jwt := auth.JWTMiddleware(tokenSvc)
	admin := auth.RequireRole(auth.RoleAdmin)

	// ── Checkout (Stripe Managed Payments) ────────────────────────────────────
	if checkoutH != nil {
		mux.Handle("POST /api/v1/checkout/product", jwt(admin(http.HandlerFunc(checkoutH.CreateProduct))))
		mux.Handle("POST /api/v1/checkout/session", jwt(http.HandlerFunc(checkoutH.CreateSession)))
		mux.Handle("POST /api/v1/checkout/webhook", http.HandlerFunc(checkoutH.Webhook))
	}

	// ── Public auth ───────────────────────────────────────────────────────────
	mux.Handle("POST /api/v1/auth/register", auth.HandleRegister(authUc))
	mux.Handle("POST /api/v1/auth/login", auth.HandleLogin(authUc))
	mux.Handle("POST /api/v1/auth/google", auth.HandleGoogleLogin(authUc))
	mux.Handle("POST /api/v1/auth/logout", auth.HandleLogout(authUc))
	mux.Handle("POST /api/v1/auth/refresh", auth.HandleRefreshToken(authUc))

	// ── Protected auth (JWT required) ─────────────────────────────────────────
	mux.Handle("GET /api/v1/auth/me",                jwt(auth.HandleGetMe(authUc)))
	mux.Handle("POST /api/v1/auth/logout/all",       jwt(auth.HandleLogoutAll(authUc)))
	mux.Handle("POST /api/v1/auth/password/change",  jwt(auth.HandleChangePassword(authUc)))
	mux.Handle("POST /api/v1/auth/otp/phone/send",   jwt(auth.HandleSendPhoneOTP(authUc)))
	mux.Handle("POST /api/v1/auth/otp/phone/verify", jwt(auth.HandleVerifyPhoneOTP(authUc)))
	mux.Handle("POST /api/v1/auth/pin/set", jwt(auth.HandleSetTradingPIN(authUc)))
	mux.Handle("POST /api/v1/auth/pin/verify", jwt(auth.HandleVerifyTradingPIN(authUc)))

	// ── 2FA (TOTP) ────────────────────────────────────────────────────────────
	mux.Handle("POST /api/v1/auth/login/2fa", auth.HandleLoginTwoFA(authUc, tokenSvc))
	mux.Handle("POST /api/v1/auth/2fa/setup", jwt(auth.HandleTwoFASetup(authUc)))
	mux.Handle("POST /api/v1/auth/2fa/enable", jwt(auth.HandleTwoFAEnable(authUc)))
	mux.Handle("POST /api/v1/auth/2fa/disable", jwt(auth.HandleTwoFADisable(authUc)))

	// ── KYC — user facing (JWT required) ──────────────────────────────────────
	mux.Handle("POST /api/v1/kyc/submit", jwt(http.HandlerFunc(kycH.HandleSubmit)))
	mux.Handle("GET /api/v1/kyc/status", jwt(http.HandlerFunc(kycH.HandleGetStatus)))

	// ── KYC — Didit identity verification ─────────────────────────────────────
	mux.Handle("POST /api/v1/kyc/didit/session", jwt(http.HandlerFunc(kycH.HandleStartDiditVerification)))
	// Webhook is public; authenticated by the X-Signature-V2 HMAC, not JWT.
	mux.Handle("POST /api/v1/kyc/didit/webhook", http.HandlerFunc(kycH.HandleDiditWebhook))

	// ── KYC — admin facing (JWT + admin role) ─────────────────────────────────
	mux.Handle("GET /api/v1/admin/kyc", jwt(admin(http.HandlerFunc(kycH.HandleAdminList))))
	mux.Handle("GET /api/v1/admin/kyc/{id}", jwt(admin(http.HandlerFunc(kycH.HandleAdminGet))))
	mux.Handle("POST /api/v1/admin/kyc/{id}/approve", jwt(admin(http.HandlerFunc(kycH.HandleAdminApprove))))
	mux.Handle("POST /api/v1/admin/kyc/{id}/reject", jwt(admin(http.HandlerFunc(kycH.HandleAdminReject))))

	// ── Orders (JWT required) ─────────────────────────────────────────────────
	mux.Handle("POST /api/v1/orders", jwt(order.HandleOrder(orderSvc)))
	mux.Handle("DELETE /api/v1/orders/{orderID}", jwt(order.HandleCancelOrder(orderSvc)))

	// ── Payments — Stripe PaymentIntent (embedded Elements flow) ──────────────
	// The frontend's DepositPanel posts here to get a client_secret for Stripe.js.
	mux.Handle("POST /api/v1/payments/deposit/intent", jwt(wallet.HandleCreateDepositIntent(walletUc)))
	// Stripe webhook alias (mirrors /wallet/deposit/webhook) — signature-verified, public.
	mux.Handle("POST /api/v1/payments/webhook",        wallet.HandleDepositWebhook(walletUc, cfg.Stripe.WebhookSecret))

	// ── Wallet — user facing (JWT required) ───────────────────────────────────
	mux.Handle("GET /api/v1/wallet/balances", jwt(wallet.HandleGetBalances(walletUc)))
	mux.Handle("GET /api/v1/wallet/balances/{currency}", jwt(wallet.HandleGetBalance(walletUc)))
	// Singular alias — the frontend calls GET /wallet/balance/{currency}.
	mux.Handle("GET /api/v1/wallet/balance/{currency}",  jwt(wallet.HandleGetBalance(walletUc)))
	mux.Handle("POST /api/v1/wallet/deposit",            jwt(wallet.HandleInitiateDeposit(walletUc)))
	mux.Handle("POST /api/v1/wallet/withdraw",           jwt(wallet.HandleInitiateWithdrawal(walletUc)))
	mux.Handle("GET /api/v1/wallet/transactions",        jwt(wallet.HandleGetTransactions(walletUc)))
	mux.Handle("POST /api/v1/wallet/transfer",           jwt(wallet.HandleTransfer(walletUc)))

	// ── Wallet — Crypto (JWT required) ────────────────────────────────────────
	mux.Handle("GET /api/v1/wallet/crypto/address/{currency}", jwt(wallet.HandleGetCryptoAddress(cryptoWalletUc)))
	mux.Handle("GET /api/v1/wallet/crypto/assets", jwt(wallet.HandleGetCryptoAssets(cryptoWalletUc)))
	mux.Handle("POST /api/v1/wallet/crypto/withdraw", jwt(wallet.HandleCryptoWithdrawal(walletUc)))

	// ── Wallet — payment provider webhook (public; verify signature in prod) ───
	mux.Handle("POST /api/v1/wallet/deposit/webhook", wallet.HandleDepositWebhook(walletUc, cfg.Stripe.WebhookSecret))
	mux.Handle("POST /api/v1/wallet/crypto/webhook", wallet.HandleCryptoWebhook(cryptoWalletUc))

	// ── Wallet — admin facing (JWT + admin role) ──────────────────────────────
	mux.Handle("GET /api/v1/admin/wallet/withdrawals", jwt(admin(wallet.HandleAdminListWithdrawals(adminWalletUc))))
	mux.Handle("POST /api/v1/admin/wallet/withdrawals/{withdrawal_id}/approve", jwt(admin(wallet.HandleAdminApproveWithdrawal(adminWalletUc))))
	mux.Handle("POST /api/v1/admin/wallet/withdrawals/{withdrawal_id}/reject", jwt(admin(wallet.HandleAdminRejectWithdrawal(adminWalletUc))))

	// ── Market data (public) ──────────────────────────────────────────────────
	mux.Handle("/api/v1/market/ws", market.HandleWebSocket(marketHub))
	mux.Handle("GET /api/v1/market/orderbook/{pairID}", order.HandleOrderBook(orderSvc))
	mux.Handle("GET /api/v1/market/klines", market.HandleKlines(binance))
	mux.Handle("GET /api/v1/market/tickers", market.HandleTickers(tickerCache))

	// ── P2P marketplace (on-chain escrow) ─────────────────────────────────────
	// Advertisements
	mux.Handle("POST /api/v1/p2p/ads",            jwt(http.HandlerFunc(p2pH.CreateAd)))
	mux.Handle("GET /api/v1/p2p/ads",             jwt(http.HandlerFunc(p2pH.ListAds)))
	mux.Handle("GET /api/v1/p2p/ads/mine",        jwt(http.HandlerFunc(p2pH.MyAds)))
	mux.Handle("GET /api/v1/p2p/ads/{id}",        jwt(http.HandlerFunc(p2pH.GetAd)))
	mux.Handle("POST /api/v1/p2p/ads/{id}/status", jwt(http.HandlerFunc(p2pH.SetAdStatus)))

	// Orders + escrow lifecycle
	mux.Handle("POST /api/v1/p2p/orders",                jwt(http.HandlerFunc(p2pH.CreateOrder)))
	mux.Handle("GET /api/v1/p2p/orders/mine",            jwt(http.HandlerFunc(p2pH.MyOrders)))
	mux.Handle("GET /api/v1/p2p/orders/{id}",            jwt(http.HandlerFunc(p2pH.GetOrder)))
	mux.Handle("GET /api/v1/p2p/orders/{id}/escrow",     jwt(http.HandlerFunc(p2pH.EscrowInfo)))
	mux.Handle("POST /api/v1/p2p/orders/{id}/paid",      jwt(http.HandlerFunc(p2pH.MarkPaid)))
	mux.Handle("POST /api/v1/p2p/orders/{id}/release",   jwt(http.HandlerFunc(p2pH.ReleaseOrder)))
	mux.Handle("POST /api/v1/p2p/orders/{id}/cancel",    jwt(http.HandlerFunc(p2pH.CancelOrder)))
	mux.Handle("POST /api/v1/p2p/orders/{id}/dispute",   jwt(http.HandlerFunc(p2pH.OpenDispute)))

	// P2P — admin dispute resolution (JWT + admin role)
	mux.Handle("GET /api/v1/admin/p2p/disputes",                  jwt(admin(http.HandlerFunc(p2pH.AdminListDisputes))))
	mux.Handle("POST /api/v1/admin/p2p/disputes/{id}/resolve",    jwt(admin(http.HandlerFunc(p2pH.AdminResolveDispute))))
}
