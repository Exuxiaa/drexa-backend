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

// NotificationService abstracts user-facing notifications (non-OTP).
type NotificationService interface {
	SendPasswordChanged(ctx context.Context, userID, email string) error
	SendNewLogin(ctx context.Context, userID, email, userAgent, ipAddress string) error
}
