package auth

import "context"

// AuthUsecase handles all user-facing authentication flows.
type AuthUsecase interface {
<<<<<<< HEAD
	// Registration & login
	Register(ctx context.Context, email, phone, password string) (*User, error)
	Login(ctx context.Context, email, password string) (*AuthToken, error)
=======
	// Register creates a new user with email + password, then issues a token pair
	Register(ctx context.Context, email, password, username string) (*AuthToken, error)

	// Login verifies email + password and issues a token pair
	Login(ctx context.Context, email, password string) (*AuthToken, error)

	// Phone verification — backend handles phone OTP for trading compliance
	SendPhoneVerificationOTP(ctx context.Context, userID string) error
	VerifyPhone(ctx context.Context, userID, otp string) (bool, error)
>>>>>>> e448e44364a4225c0819ff59d6af60c71d778498

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

