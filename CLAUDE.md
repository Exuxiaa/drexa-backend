# Drexa Backend — Context

> **This repository contains only the Go backend service.**

## Key Technical Decisions

- **Architecture**: Modular Monolith — satu binary, domain dipisah ketat di `internal/`, siap di-extract ke microservice nanti
- **Language**: Go
- **Auth**: Custom email/password auth, JWT access token (15m) + opaque refresh token (7d, stored hashed in PG); no Firebase
- **Database**: PostgreSQL (primary), golang-migrate untuk versioned migrations; no MySQL, no Redis
- **Cache**: In-memory Go (price cache, rate limiting, order book); PostgreSQL (OTP, token blacklist, idempotency keys)
- **Config**: Viper + godotenv, single `.env` file, secrets via env injection
- **Logging**: zerolog structured logging, request ID propagation
- **Market data**: Binance WS / CoinGecko API — TIDAK dipakai langsung sebagai harga eksekusi
- **Payments**: Xendit/Midtrans (fiat IDR), crypto HD wallet per user; no Stripe
- **Notifications**: Twilio (SMS), SendGrid (email)
- **KYC**: `internal/kyc/` domain — Submission state machine (pending → approved/rejected); narrow `kyc.UserService` adapter prevents circular import with auth; mock provider
- **2FA**: TOTP (pquerna/otp); setup/enable/disable via protected endpoints; login returns challenge_token when 2FA enabled; challenge token has Scope="2fa_challenge" and is rejected by JWTMiddleware
- **RBAC**: RequireRole middleware + RequireKycLevel(minLevel) middleware in auth/middleware.go
- **Deployment**: Local only — single instance, jalankan dengan `go run ./cmd/server`

---

## Class Diagram Entities (canonical)

| Domain   | Entities |
|----------|----------|
| auth     | User, RefreshToken, OTPCode, AuthToken |
| kyc      | KycSubmission (in `internal/kyc/`) |
| wallet   | Wallet, LedgerEntry, Transaction, DepositAddress |
| market   | Coin, TradingPair, PriceSnapshot, Candle |
| order    | Order, Trade |
| p2p      | P2PAdvertisement, P2POrder, P2PDispute |

### User
`userId, email, phone, passwordHash, tradingPINHash, role(user/merchant/admin), kycLevel(int), twoFAEnabled, twoFASecret, createdAt, updatedAt`

### KycSubmission
`submissionId, userId, status(pending/approved/rejected), fullName, idNumber(encrypted NIK), idType, fileUrl, selfieUrl, rejectionReason, submittedAt, reviewedBy, reviewedAt`

### Wallet
`walletId, userId, walletAddress, currency, availableBalance, lockedBalance, createdAt`

### LedgerEntry
`entryId, walletId, type(debit/credit), amount, currency, refType, refId, description, createdAt`

### Transaction
`transactionId, userId, type(deposit/withdrawal/trade_buy/trade_sell/fee/p2p_escrow/p2p_release), amount, currency, status(pending/confirmed/failed), txHash, fee, createdAt`

### Order
`orderId, userId, pairId, side(buy/sell), type(market/limit), status(pending/open/partially_filled/filled/cancelled), price, quantity, filledQuantity, lockedAmount, fee, createdAt, updatedAt`

### Trade
`tradeId, pairId, makerOrderId, takerOrderId, price, quantity, makerFee, takerFee, executedAt`

### P2POrder
`p2pOrderId, advertisementId, buyerId, sellerId, amount, totalIDR, status(created/paid/released/disputed/cancelled), paymentProofUrl, escrowWalletId, createdAt, paidAt, releasedAt, expiredAt`

---

## Struktur Project
```
drexa-backend/
├── cmd/
│   └── server/
│       ├── main.go          # entry point
│       ├── server.go        # wire semua dependencies
│       └── routes.go        # register semua route
│
├── internal/
│   ├── auth/                # register, login, JWT, 2FA TOTP, trading PIN, phone OTP
│   │   ├── domain.go        # User, RefreshToken, OTPCode, AuthToken, TwoFASetup + all interfaces
│   │   ├── handler.go       # HTTP handlers; 2FA: setup/enable/disable/login
│   │   ├── middleware.go    # JWTMiddleware (rejects scoped tokens), RequireRole, RequireKycLevel
│   │   ├── usecase/
│   │   │   └── auth_usecase.go  # register/login/2FA/PIN/OTP
│   │   ├── repository/
│   │   │   ├── user_repository.go
│   │   │   ├── refresh_token_repository.go
│   │   │   └── otp_repository.go
│   │   └── service/
│   │       ├── token_service.go        # GenerateTwoFAChallengeToken (scope="2fa_challenge")
│   │       ├── otp_service.go          # PG-backed, bcrypt-hashed OTP codes
│   │       ├── notification_service.go # mock
│   │       ├── notification_sendgrid.go
│   │       ├── email_sendgrid.go
│   │       └── sms_twilio.go
│   │
│   ├── kyc/                 # extracted Fase 1.2 — full domain, repo, usecase, handler
│   │   ├── domain.go        # Submission, UserSnapshot, all kyc interfaces
│   │   ├── handler.go       # submit, status, admin list/approve/reject
│   │   ├── repository/
│   │   │   └── kyc_repository.go
│   │   ├── service/
│   │   │   └── notification_service.go  # mock KYC notifications
│   │   └── usecase/
│   │       ├── kyc_usecase.go
│   │       └── admin_kyc_usecase.go
│   ├── wallet/              # domain.go — full impl in Fase 2
│   ├── market/              # domain.go — full impl in Fase 3
│   ├── order/               # domain.go — full impl in Fase 4A
│   ├── p2p/                 # domain.go — full impl in Fase 4B
│   │
│   └── platform/            # infrastruktur shared, bukan domain bisnis
│       ├── postgres/        # GORM + pgx connection
│       ├── migrate/         # golang-migrate runner
│       └── middleware/      # request ID injection
│
├── pkg/                     # reusable, zero domain knowledge
│   ├── config/              # Viper loader
│   ├── logger/              # zerolog setup
│   ├── jwt/                 # sign + verify (HS256)
│   ├── password/            # bcrypt hash + check
│   └── apperr/              # typed sentinel errors
│
├── migrations/
│   ├── 000001_auth.{up,down}.sql     # users, refresh_tokens, otp_codes
│   ├── 000002_kyc.{up,down}.sql      # kyc_submissions
│   ├── 000003_wallet.{up,down}.sql   # wallets, ledger_entries, transactions, deposit_addresses
│   ├── 000004_market.{up,down}.sql   # coins, trading_pairs, price_snapshots, candles
│   ├── 000005_orders.{up,down}.sql   # orders, trades
│   └── 000006_p2p.{up,down}.sql      # p2p_advertisements, p2p_orders, p2p_disputes
│
├── .env
├── go.mod
└── CLAUDE.md
```

## Local Development

```bash
# Start PostgreSQL
docker compose up postgres -d

# Run server (auto-migrates on startup)
go run ./cmd/server

# Or run everything with Docker
docker compose up
```

PostgreSQL runs on port `5432`. Server starts on `:8080`.

---

## Project Rebuild TODO

> Status legend: `[ ]` belum · `[x]` selesai · `[-]` skip/ditunda

### Fase 0 — Fondasi

- [x] **0.1 Struktur folder & arsitektur** — `cmd/server/`, `internal/`, `pkg/`; domain-first layout; Modular Monolith
- [x] **0.2 Config & environment** — Viper + godotenv, `.env`, all secrets via env; Firebase/Redis/Stripe removed
- [x] **0.3 Database & migrations** — PostgreSQL; golang-migrate; 6 versioned migration files (000001–000006); auto-run on startup
- [x] **0.4 Logging & observability** — zerolog structured logging; request ID middleware (`X-Request-ID`)

### Fase 1 — Identity & Access

- [x] **1.1 Auth domain** — register, login, logout; email/password; JWT (15m) + opaque refresh token (7d, PG-hashed); phone OTP (PG-backed, bcrypt-hashed); trading PIN
- [x] **1.2 KYC domain** — `internal/kyc/` with full domain/repo/usecase/handler; `kyc.UserService` narrow interface adapter (no circular import); user submit + admin approve/reject; mock notifications
- [x] **1.3 RBAC & permission layer** — `RequireRole` + `RequireKycLevel(minLevel)` middleware; admin KYC routes gated with both
- [x] **1.4 2FA TOTP** — pquerna/otp; setup/enable/disable flows; login returns challenge_token when 2FA enabled; `/auth/login/2fa` validates challenge_token (scope check) + TOTP code; JWTMiddleware rejects scoped tokens

### Fase 2 — Wallet & Ledger

- [ ] **2.1 Internal ledger (double-entry)** — atomic debit/credit; race condition prevention
- [ ] **2.2 Multi-currency wallet** — available vs locked balance; lock on order submit
- [ ] **2.3 Crypto deposit & withdrawal** — HD wallet derivation; DepositAddress entity already in schema
- [ ] **2.4 Fiat deposit & withdrawal (IDR)** — Xendit/Midtrans; webhook handler; idempotency

### Fase 3 — Market Data & Price Feed

- [ ] **3.1 External price feed** — Binance WS; OHLCV candles
- [ ] **3.2 Real-time price distribution** — Go WebSocket hub
- [ ] **3.3 Coin & pair management** — admin CRUD; Coin/TradingPair already in schema
- [ ] **3.4 Caching layer** — in-memory TTL map (no Redis)

### Fase 4A — CEX Matching Engine

- [ ] **4A.1 Order book & matching engine** — in-memory, price-time priority, self-trade prevention
- [ ] **4A.2 Order lifecycle management** — balance lock/unlock
- [ ] **4A.3 Trade execution & settlement** — atomic ledger entries; Trade entity in schema
- [ ] **4A.4 Order book WebSocket feed** — diff publish + sequence numbers

### Fase 4B — P2P Marketplace

- [ ] **4B.1 Advertisement system** — P2PAdvertisement in schema; handlers TODO
- [ ] **4B.2 Escrow engine** — P2POrder + escrow wallet; auto-cancel on timeout
- [ ] **4B.3 Payment confirmation flow** — buyer → paid → seller confirm → release
- [ ] **4B.4 Dispute & resolution system** — P2PDispute in schema; admin resolution

### Fase 5 — Production Hardening

- [ ] **5.1 Rate limiting & anti-abuse** — in-memory token bucket; no Redis needed
- [ ] **5.2 Idempotency** — `idempotency_keys` PG table; header-based
- [ ] **5.3 Audit log & compliance trail** — immutable financial action log
- [ ] **5.4 Security hardening** — input validation; no secrets in logs; withdrawal whitelist
