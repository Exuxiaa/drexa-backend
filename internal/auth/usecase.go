package auth

import "context"

// AuthUsecase handles all user-facing authentication flows.
type AuthUsecase interface {
	// Registration & login
	Register(ctx context.Context, email, phone, password string) (*User, error)
	Login(ctx context.Context, email, password string) (*AuthToken, error)

	// Session management
	RefreshToken(ctx context.Context, rawRefreshToken string) (*AuthToken, error)
	Logout(ctx context.Context, rawRefreshToken string) error
	LogoutAll(ctx context.Context, userID string) error

	// Credential management
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error

	// Phone OTP — used during onboarding and sensitive actions
	SendPhoneOTP(ctx context.Context, userID string) error
	VerifyPhoneOTP(ctx context.Context, userID, otp string) error

	// Trading PIN — required before executing trades or withdrawals
	SetTradingPIN(ctx context.Context, userID, pin string) error
	VerifyTradingPIN(ctx context.Context, userID, pin string) (bool, error)

	// Two-factor authentication (TOTP)
	InitiateTwoFA(ctx context.Context, userID string) (*TwoFASetup, error)
	ConfirmTwoFA(ctx context.Context, userID, code string) error
	DisableTwoFA(ctx context.Context, userID, code string) error
	VerifyTwoFA(ctx context.Context, userID, code string) (*AuthToken, error)
}

