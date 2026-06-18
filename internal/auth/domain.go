package auth

import (
	"errors"
	"time"
)

// ─── Enums ───────────────────────────────────────────────────────────────────

type UserRole string

const (
	RoleUser     UserRole = "user"
	RoleMerchant UserRole = "merchant"
	RoleAdmin    UserRole = "admin"
)

// ─── Entities ────────────────────────────────────────────────────────────────

type User struct {
	UserID         string    `gorm:"primaryKey;column:user_id"`
	Email          string    `gorm:"column:email;uniqueIndex"`
	Phone          string    `gorm:"column:phone;uniqueIndex"`
	PasswordHash   string    `gorm:"column:password_hash"`
	TradingPINHash string    `gorm:"column:trading_pin_hash;default:''"`
	Role           UserRole  `gorm:"column:role;default:user"`
	KycLevel       int       `gorm:"column:kyc_level;default:0"`
	TwoFAEnabled   bool      `gorm:"column:two_fa_enabled;default:false"`
	TwoFASecret    string    `gorm:"column:two_fa_secret;default:''"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// RefreshToken represents a persisted refresh token for session management.
type RefreshToken struct {
	TokenID   string     `gorm:"primaryKey;column:token_id"`
	UserID    string     `gorm:"column:user_id;index"`
	TokenHash string     `gorm:"column:token_hash;uniqueIndex"`
	UserAgent string     `gorm:"column:user_agent"`
	IPAddress string     `gorm:"column:ip_address"`
	ExpiredAt time.Time  `gorm:"column:expired_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
}

// OTPCode is a short-lived one-time password stored in PostgreSQL.
type OTPCode struct {
	OTPID     string     `gorm:"primaryKey;column:otp_id"`
	Key       string     `gorm:"column:key;uniqueIndex"` // e.g. "phone:+6281234567890"
	CodeHash  string     `gorm:"column:code_hash"`       // bcrypt hash of the 6-digit code
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
}

// AuthToken is the response payload after successful authentication.
// It is never persisted — access token is a stateless JWT, refresh token is stored hashed.
// When RequiresTwoFA is true, only ChallengeToken is populated; AccessToken/RefreshToken are empty.
type AuthToken struct {
	AccessToken    string
	RefreshToken   string
	TokenType      string    // always "Bearer"
	ExpiresAt      time.Time // when the AccessToken expires
	RequiresTwoFA  bool
	ChallengeToken string
}

// JWTClaims is the payload embedded inside every access token.
type JWTClaims struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Role      UserRole  `json:"role"`
	KycLevel  int       `json:"kyc_level"`
	Scope     string    `json:"scope,omitempty"` // non-empty for restricted tokens like "2fa_challenge"
	ExpiresAt time.Time `json:"exp"`
	IssuedAt  time.Time `json:"iat"`
}

// TwoFASetup carries the TOTP provisioning data shown to the user once.
type TwoFASetup struct {
	Secret    string // base32 TOTP secret for manual entry
	QRCodeURL string // otpauth:// URI for QR code generation
}

// ─── Domain Errors ───────────────────────────────────────────────────────────

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrPhoneAlreadyExists = errors.New("phone already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")

	ErrTokenInvalid = errors.New("token is invalid")
	ErrTokenExpired = errors.New("token has expired")

	ErrOTPInvalid = errors.New("otp is invalid or expired")

	ErrPINInvalid = errors.New("trading pin is invalid")
)
