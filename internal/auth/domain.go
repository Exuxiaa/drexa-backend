package auth

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ─── Entities ───────────────────────────────────────────────────────────────

type User struct {
	UserID          string         `gorm:"primaryKey;column:user_id;size:36"`
	FirebaseUID     string         `gorm:"column:firebase_uid;size:128;uniqueIndex"`
	UserName        string         `gorm:"column:username;size:191"`
	Email           string         `gorm:"column:email;size:191;uniqueIndex"`
	PhoneNumber     string         `gorm:"column:phone_number;size:32"`
	TradingPinHash  string         `gorm:"column:trading_pin_hash;size:255"`
	IsEmailVerified bool           `gorm:"column:is_email_verified;default:false"`
	IsPhoneVerified bool           `gorm:"column:is_phone_verified;default:false"`
	LastLoginAt     time.Time      `gorm:"column:last_login_at"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	ModifiedAt      time.Time      `gorm:"column:modified_at;autoUpdateTime"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at;index"` // soft delete — required for OJK audit trail

	KycProfile KycProfile `gorm:"foreignKey:UserID"`
}

// KycStatus is a defined type to prevent arbitrary string values
type KycStatus string

const (
	KycStatusPending  KycStatus = "pending"
	KycStatusApproved KycStatus = "approved"
	KycStatusRejected KycStatus = "rejected"
	KycStatusExpired  KycStatus = "expired"
)

type KycAddress struct {
	Street    string `gorm:"column:address_street;size:255"`
	RTRW      string `gorm:"column:address_rt_rw;size:16"`
	Kelurahan string `gorm:"column:address_kelurahan;size:100"`
	Kecamatan string `gorm:"column:address_kecamatan;size:100"`
	Kabupaten string `gorm:"column:address_kabupaten;size:100"`
	Provinsi  string `gorm:"column:address_provinsi;size:100"`
	KodePos   string `gorm:"column:address_kode_pos;size:16"`
}

type KycProfile struct {
	KycID              string     `gorm:"primaryKey;column:kyc_id;size:36"`
	UserID             string     `gorm:"column:user_id;size:36;uniqueIndex"` // FK to users, one-to-one
	NIKEncrypted       string     `gorm:"column:nik_encrypted"`               // AES-256 encrypted, not hashed
	FullName           string     `gorm:"column:full_name;size:191"`
	DateOfBirth        time.Time  `gorm:"column:date_of_birth"`
	Address            KycAddress `gorm:"embedded"`                               // flattened into kyc_profiles table using column tags above
	DocumentImagePath  string     `gorm:"column:document_image_path;size:255"`    // path/URL to encrypted object storage
	FaceImagePath      string     `gorm:"column:face_image_path;size:255"`        // path/URL to encrypted object storage
	VerificationSource string     `gorm:"column:verification_source;size:64"`     // e.g. "verihubs", "vida"
	DukcapilVerified   bool       `gorm:"column:dukcapil_verified;default:false"` // was NIK validated against Dukcapil?
	RejectionReason    string     `gorm:"column:rejection_reason;size:255"`       // populated if status = rejected
	ReviewedBy         string     `gorm:"column:reviewed_by;size:36"`             // admin user ID who approved or rejected
	SubmittedAt        time.Time  `gorm:"column:submitted_at"`
	VerifiedAt         *time.Time `gorm:"column:verified_at"` // pointer — nil until approved
	ExpiresAt          *time.Time `gorm:"column:expires_at"`  // KYC can expire and require re-verification
	Status             KycStatus  `gorm:"column:status;default:pending"`
	CreatedAt          time.Time  `gorm:"column:created_at;autoCreateTime"`
	ModifiedAt         time.Time  `gorm:"column:modified_at;autoUpdateTime"`
}

type AuthToken struct {
	AccessToken  string
	RefreshToken string
	TokenType    string    // always "Bearer"
	ExpiresAt    time.Time // when the AccessToken expires — lets client proactively refresh
	// note: AuthToken is never persisted — no GORM tags needed
	// AccessToken is a stateless JWT, RefreshToken is stored separately in refresh_tokens table
}

// RefreshToken represents a persisted refresh token for session management
type RefreshToken struct {
	TokenID   string     `gorm:"primaryKey;column:token_id;size:36"`
	UserID    string     `gorm:"column:user_id;size:36;index"`          // FK to users
	TokenHash string     `gorm:"column:token_hash;size:64;uniqueIndex"` // store hashed, never plaintext
	UserAgent string     `gorm:"column:user_agent;size:255"`            // which device/browser issued this token
	IPAddress string     `gorm:"column:ip_address;size:45"`             // for suspicious activity detection
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
	RevokedAt *time.Time `gorm:"column:revoked_at"` // nil if still valid, set on logout
}

type PasswordResetToken struct {
	TokenID   string     `gorm:"primaryKey;column:token_id;size:36"`
	UserID    string     `gorm:"column:user_id;size:36;index"`          // FK to users
	TokenHash string     `gorm:"column:token_hash;size:64;uniqueIndex"` // hashed before storage
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at"` // nil until redeemed
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
}

// FirebaseClaims holds the verified identity extracted from a Firebase ID token.
type FirebaseClaims struct {
	UID           string
	Email         string
	EmailVerified bool
	Provider      string // e.g. "google.com", "apple.com", "password"
}

// ─── Domain Errors ───────────────────────────────────────────────────────────

var (
	// User
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyExists = errors.New("email already exists")

	// Token
	ErrTokenInvalid = errors.New("token is invalid")
	ErrTokenExpired = errors.New("token has expired")

	// KYC
	ErrKycNotFound         = errors.New("kyc profile not found")
	ErrKycAlreadySubmitted = errors.New("kyc already submitted")
	ErrKycNotApproved      = errors.New("kyc not approved")

	// OTP
	ErrOTPInvalid = errors.New("otp is invalid")
	ErrOTPExpired = errors.New("otp has expired")
)
