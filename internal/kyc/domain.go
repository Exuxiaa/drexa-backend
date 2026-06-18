package kyc

import (
	"context"
	"errors"
	"time"
)

// ─── Enums ───────────────────────────────────────────────────────────────────

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// ─── Entities ────────────────────────────────────────────────────────────────

type Submission struct {
	SubmissionID    string     `gorm:"primaryKey;column:submission_id"`
	UserID          string     `gorm:"column:user_id;index"`
	Status          Status     `gorm:"column:status;default:pending"`
	FullName        string     `gorm:"column:full_name"`
	IDNumber        string     `gorm:"column:id_number"`  // AES-256 encrypted NIK
	IDType          string     `gorm:"column:id_type"`
	FileURL         string     `gorm:"column:file_url"`
	SelfieURL       string     `gorm:"column:selfie_url"`
	RejectionReason *string    `gorm:"column:rejection_reason"`
	SubmittedAt     time.Time  `gorm:"column:submitted_at"`
	ReviewedBy      string     `gorm:"column:reviewed_by;default:''"`
	ReviewedAt      *time.Time `gorm:"column:reviewed_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

// UserSnapshot is the minimal user data KYC needs.
// Avoids a direct import of internal/auth.
type UserSnapshot struct {
	UserID string
	Email  string
}

// ─── Repository Interface ─────────────────────────────────────────────────────

type Repository interface {
	Create(ctx context.Context, sub *Submission) error
	Update(ctx context.Context, sub *Submission) error
	UpdateStatus(ctx context.Context, submissionID string, status Status, reason, reviewedBy string) error

	FindByID(ctx context.Context, submissionID string) (*Submission, error)
	FindLatestByUserID(ctx context.Context, userID string) (*Submission, error)
	FindByStatus(ctx context.Context, status Status) ([]Submission, error)
}

// ─── Service Interfaces ───────────────────────────────────────────────────────

// UserService is the narrow interface KYC needs from the auth domain.
// Satisfied by the auth UserRepository adapter in cmd/server.
type UserService interface {
	FindByID(ctx context.Context, userID string) (*UserSnapshot, error)
	UpdateKycLevel(ctx context.Context, userID, reviewedBy string, level int) error
}

// StorageService uploads KYC documents and returns a persisted URL.
type StorageService interface {
	Upload(ctx context.Context, userID, docType string, data []byte) (string, error)
}

// NotificationService sends KYC decision emails.
type NotificationService interface {
	SendKycApproved(ctx context.Context, userID, email string) error
	SendKycRejected(ctx context.Context, userID, email, reason string) error
}

// ─── Usecase Interfaces ───────────────────────────────────────────────────────

type Usecase interface {
	Submit(ctx context.Context, userID string, sub *Submission) error
	GetByUserID(ctx context.Context, userID string) (*Submission, error)
	IsApproved(ctx context.Context, userID string) (bool, error)
}

type AdminUsecase interface {
	ListByStatus(ctx context.Context, status Status) ([]Submission, error)
	GetByID(ctx context.Context, submissionID string) (*Submission, error)
	Approve(ctx context.Context, submissionID, reviewedBy string) error
	Reject(ctx context.Context, submissionID, reviewedBy, reason string) error
}

// ─── Domain Errors ───────────────────────────────────────────────────────────

var (
	ErrNotFound         = errors.New("kyc submission not found")
	ErrAlreadySubmitted = errors.New("a pending kyc submission already exists")
	ErrNotApproved      = errors.New("kyc not approved")
	ErrUserNotFound     = errors.New("user not found")
)
