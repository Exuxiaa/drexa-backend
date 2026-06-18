# Drexa Backend — API Reference

Base URL: `http://localhost:8080/api/v1`

---

## Authentication

Protected endpoints require a valid JWT access token. Send it in one of two ways:

| Method | Value |
|--------|-------|
| Header | `Authorization: Bearer <access_token>` |
| Cookie | `access_token=<access_token>` |

Tokens are issued at sign-in and refreshed via `/auth/refresh`. The server sets `HttpOnly` cookies automatically on sign-in/refresh responses.

---

## Common Responses

All responses are JSON.

```json
{ "message": "..." }   // success
{ "error": "..." }     // failure
```

| Status | Meaning |
|--------|---------|
| `200` | OK |
| `201` | Created |
| `400` | Bad request / validation error |
| `401` | Unauthorized / invalid or expired token |
| `404` | Resource not found |
| `422` | Unprocessable entity (e.g. insufficient funds) |
| `500` | Internal server error |

---

## Auth

### Sign In

```
POST /auth/signin
```

Authenticates using a Firebase ID token obtained from the frontend Firebase SDK after the user signs in with Google, Apple, or any configured provider. On the first call for a given Firebase UID, a user record is automatically created. Sets `access_token` and `refresh_token` `HttpOnly` cookies on success.

**Request**
```json
{
  "id_token": "firebase_id_token_from_frontend"
}
```

**Response `200`**
```json
{ "message": "sign-in successful" }
```

---

### Logout

```
POST /auth/logout
```

Revokes the current refresh token. Reads the `refresh_token` cookie — no request body required.

**Response `200`**
```json
{ "message": "logged out" }
```

---

### Refresh Token

```
POST /auth/refresh
```

Issues a new access token and rotates the refresh token. Reads the `refresh_token` cookie. Sets new `access_token` and `refresh_token` cookies on success.

**Response `200`**
```json
{ "message": "token refreshed" }
```

**Response `401`** — missing, invalid, or expired refresh token
```json
{ "error": "invalid or expired token" }
```

---

### Logout All Devices

```
POST /auth/logout/all
```

🔒 **Protected**

Revokes all active sessions for the authenticated user.

**Response `200`**
```json
{ "message": "all sessions revoked" }
```

---

### Verify Phone

```
POST /auth/verify/phone
```

🔒 **Protected**

Verifies the OTP sent to the user's phone number via SMS.

**Request**
```json
{
  "user_id": "uuid",
  "otp": "123456"
}
```

**Response `200`**
```json
{ "message": "phone verified" }
```

**Response `401`** — wrong or expired OTP
```json
{ "error": "invalid or expired OTP" }
```

---

## Trading PIN

### Set PIN

```
POST /auth/pin/set
```

🔒 **Protected**

Sets or updates the trading PIN. Required before executing trades or withdrawals.

**Request**
```json
{
  "pin": "123456"
}
```

**Response `200`**
```json
{ "message": "trading pin set" }
```

---

### Verify PIN

```
POST /auth/pin/verify
```

🔒 **Protected**

Verifies the trading PIN before a sensitive action (e.g. placing a trade or withdrawing funds).

**Request**
```json
{
  "pin": "123456"
}
```

**Response `200`**
```json
{ "message": "pin verified" }
```

**Response `401`** — wrong PIN
```json
{ "error": "invalid pin" }
```

---

## Payments

### Create Deposit Intent

```
POST /payments/deposit/intent
```

🔒 **Protected**

Creates a Stripe PaymentIntent. The frontend uses the returned `client_secret` with Stripe.js to confirm the payment. A pending transaction record is created immediately with the returned `tx_id`.

Minimum deposit: **$10.00** (1000 cents).

**Request**
```json
{
  "amount": 5000
}
```

> Amounts are in the smallest currency unit (cents for USD). `5000` = $50.00.

**Response `201`**
```json
{
  "client_secret": "pi_xxx_secret_xxx",
  "tx_id": "uuid"
}
```

**Response `400`** — amount below minimum or invalid
```json
{ "error": "minimum deposit is $10" }
```

---

### Withdraw

```
POST /payments/withdraw
```

🔒 **Protected**

Debits the user's wallet balance. Minimum withdrawal: **$10.00** (1000 cents).

**Request**
```json
{
  "amount": 2000
}
```

> Amounts are in cents. `2000` = $20.00.

**Response `200`**
```json
{ "message": "withdrawal recorded" }
```

**Response `400`** — amount below minimum or invalid
```json
{ "error": "minimum withdrawal is $10" }
```

**Response `422`** — not enough balance
```json
{ "error": "insufficient funds" }
```

---

### Stripe Webhook

```
POST /payments/webhook
```

Receives events from Stripe (e.g. `payment_intent.succeeded`). **Not behind JWT** — authenticity is verified via the `Stripe-Signature` header using `STRIPE_WEBHOOK_SECRET`. Call this endpoint only from Stripe's dashboard webhook configuration.

**Headers** (set by Stripe)
```
Stripe-Signature: t=...,v1=...
```

**Response `200`** — event processed successfully

**Response `400`** — invalid signature or unreadable body

---

## Wallet

### Get Balance

```
GET /wallet/balance
```

🔒 **Protected**

Returns the user's current fiat wallet balance.

**Response `200`**
```json
{
  "balance": 15000,
  "currency": "usd"
}
```

> `balance` is in cents. `15000` = $150.00.

---

### Get Transactions

```
GET /wallet/transactions?limit=20&offset=0
```

🔒 **Protected**

Returns the user's paginated transaction history, ordered by creation date descending.

**Query Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| `limit` | integer | Number of records to return (default: 0 = all) |
| `offset` | integer | Number of records to skip (default: 0) |

**Response `200`**
```json
[
  {
    "TxID": "uuid",
    "UserID": "uuid",
    "Type": "deposit",
    "Amount": 5000,
    "Currency": "usd",
    "Status": "completed",
    "StripePaymentIntentID": "pi_xxx",
    "CreatedAt": "2024-01-15T10:30:00Z",
    "UpdatedAt": "2024-01-15T10:30:45Z"
  }
]
```

> **Type** values: `deposit`, `withdrawal`  
> **Status** values: `pending`, `completed`, `failed`  
> **Amount** is in cents.
