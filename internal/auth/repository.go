package auth

import "context"

// UserRepository handles persistence for User entities.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error

	FindByID(ctx context.Context, userID string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByPhone(ctx context.Context, phone string) (*User, error)

	UpdatePasswordHash(ctx context.Context, userID, hash string) error
	UpdateTradingPINHash(ctx context.Context, userID, hash string) error
	UpdateTwoFA(ctx context.Context, userID, secret string, enabled bool) error
	UpdateKycLevel(ctx context.Context, userID, reviewedBy string, level int) error
}

// RefreshTokenRepository handles persistence for refresh token sessions.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) error
	Revoke(ctx context.Context, tokenID string) error
	RevokeAllByUserID(ctx context.Context, userID string) error

	FindByTokenHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	FindActiveByUserID(ctx context.Context, userID string) ([]RefreshToken, error)

	DeleteExpired(ctx context.Context) error
}

// OTPRepository handles persistence for short-lived one-time passwords.
type OTPRepository interface {
	Upsert(ctx context.Context, otp *OTPCode) error         // creates or replaces for the same key
	FindByKey(ctx context.Context, key string) (*OTPCode, error)
	MarkUsed(ctx context.Context, otpID string) error
	DeleteExpired(ctx context.Context) error
}

