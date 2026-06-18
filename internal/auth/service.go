package auth

import (
	"context"
	"time"
)

// TokenService abstracts JWT generation and validation.
type TokenService interface {
	GenerateAccessToken(ctx context.Context, user *User) (string, error)
	GenerateRefreshToken(ctx context.Context, userID string) (string, error)
	// GenerateTwoFAChallengeToken returns a short-lived JWT with Scope="2fa_challenge".
	GenerateTwoFAChallengeToken(ctx context.Context, userID string) (string, error)
	ValidateAccessToken(ctx context.Context, token string) (*JWTClaims, error)
	HashToken(token string) string
	RefreshExpiration() time.Duration
}

// OTPService abstracts OTP generation, storage (PostgreSQL), and delivery.
type OTPService interface {
	// GenerateAndSendSMS generates a 6-digit OTP, stores it hashed in PG, sends via SMS.
	GenerateAndSendSMS(ctx context.Context, key, phone string) error

	// GenerateAndSendEmail generates a 6-digit OTP, stores it hashed in PG, sends via email.
	GenerateAndSendEmail(ctx context.Context, key, email string) error

	// Verify checks and consumes the OTP for key — returns false (not error) on mismatch/expiry.
	Verify(ctx context.Context, key, otp string) (bool, error)
}

<<<<<<< HEAD
// NotificationService abstracts user-facing notifications (non-OTP).
=======
// TokenService abstracts JWT generation and validation.
// Implement this with golang-jwt/jwt or any compatible library.
type TokenService interface {
	// GenerateAccessToken issues a short-lived JWT containing user claims
	GenerateAccessToken(ctx context.Context, user *User) (string, error)

	// GenerateRefreshToken issues a long-lived opaque token for session renewal
	GenerateRefreshToken(ctx context.Context, userID string) (string, error)

	// ValidateAccessToken parses and validates a JWT, returns the embedded claims
	ValidateAccessToken(ctx context.Context, token string) (*JWTClaims, error)

	// HashToken hashes a raw token before storage — use for refresh and reset tokens
	HashToken(token string) string
}

// RateLimiter throttles repeated actions keyed by an identifier (e.g. client IP).
// Backed by Redis so the limit is shared across all server instances.
type RateLimiter interface {
	// Allow increments the counter for key and reports whether the action is
	// still permitted within the given window. Returns false once limit is exceeded.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// FirebaseVerifier verifies Firebase ID tokens issued by the frontend Firebase SDK.
// Implement via FirebaseAuthService in internal/auth/service once Firebase is configured.
type FirebaseVerifier interface {
	VerifyIDToken(ctx context.Context, idToken string) (*FirebaseClaims, error)
}

// NotificationService abstracts user-facing notifications beyond OTP.
// Implement this for email/push once a provider is chosen — use MockNotificationService in the meantime.
>>>>>>> e448e44364a4225c0819ff59d6af60c71d778498
type NotificationService interface {
	SendPasswordChanged(ctx context.Context, userID, email string) error
	SendNewLogin(ctx context.Context, userID, email, userAgent, ipAddress string) error
}
