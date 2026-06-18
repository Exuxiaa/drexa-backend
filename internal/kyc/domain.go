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

	// Didit identity-verification session tracking (migration 000008).
	DiditSessionID  string `gorm:"column:didit_session_id;index"`
	DiditSessionURL string `gorm:"column:didit_session_url"`
	DiditStatus     string `gorm:"column:didit_status"` // raw Didit status literal, e.g. "Approved"
}

// UserSnapshot is the minimal user data KYC needs.
// Avoids a direct import of internal/auth.
type UserSnapshot struct {
	UserID string
	Email  string
}

// ProcessedEvent records one consumed Didit webhook delivery (idempotency).
type ProcessedEvent struct {
	EventID     string    `gorm:"primaryKey;column:event_id"`
	ProcessedAt time.Time `gorm:"column:processed_at;autoCreateTime"`
}

func (ProcessedEvent) TableName() string { return "didit_processed_events" }

// ─── Didit Verification ───────────────────────────────────────────────────────

// DiditSession is what a create-session call returns to the client.
type DiditSession struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
	Status    string `json:"status"`
}

// DiditWebhookEvent is the V3 session webhook envelope (only fields we consume).
type DiditWebhookEvent struct {
	EventID     string `json:"event_id"`
	WebhookType string `json:"webhook_type"`
	SessionID   string `json:"session_id"`
	Status      string `json:"status"`     // case-sensitive literal: "Approved", "Declined", ...
	VendorData  string `json:"vendor_data"` // our internal user id
}

// Didit session status literals (mixed-case, compared case-sensitively).
const (
	DiditApproved   = "Approved"
	DiditDeclined   = "Declined"
	DiditInReview   = "In Review"
	DiditResubmit   = "Resubmitted"
	DiditKycExpired = "Kyc Expired"
)

// ─── Repository Interface ─────────────────────────────────────────────────────

type Repository interface {
	Create(ctx context.Context, sub *Submission) error
	Update(ctx context.Context, sub *Submission) error
	UpdateStatus(ctx context.Context, submissionID string, status Status, reason, reviewedBy string) error

	FindByID(ctx context.Context, submissionID string) (*Submission, error)
	FindLatestByUserID(ctx context.Context, userID string) (*Submission, error)
	FindByStatus(ctx context.Context, status Status) ([]Submission, error)

	// Didit session tracking.
	FindByDiditSessionID(ctx context.Context, sessionID string) (*Submission, error)
	UpdateDiditResult(ctx context.Context, sessionID, diditStatus string, status Status) error

	// Webhook idempotency.
	IsEventProcessed(ctx context.Context, eventID string) (bool, error)
	MarkEventProcessed(ctx context.Context, eventID string) error
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

// DiditService is the Didit identity-verification provider.
// CreateSession is called server-side (holds the x-api-key); VerifyWebhook
// authenticates the X-Signature-V2 HMAC and enforces timestamp freshness.
type DiditService interface {
	CreateSession(ctx context.Context, vendorData string) (*DiditSession, error)
	VerifyWebhook(payload []byte, signatureV2 string, timestamp int64) error
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

// DiditUsecase drives the Didit-backed verification flow.
type DiditUsecase interface {
	// StartVerification creates a Didit session for the user and persists a
	// pending submission row tracking that session. Returns the hosted url.
	StartVerification(ctx context.Context, userID string) (*DiditSession, error)
	// HandleWebhook applies an already-verified, deduped webhook decision.
	HandleWebhook(ctx context.Context, event *DiditWebhookEvent) error
	// Service exposes the underlying provider for signature verification at the edge.
	Service() DiditService
	// Repo exposes idempotency checks at the edge before dispatch.
	Repo() Repository
}

// ─── Domain Errors ───────────────────────────────────────────────────────────

var (
	ErrNotFound         = errors.New("kyc submission not found")
	ErrAlreadySubmitted = errors.New("a pending kyc submission already exists")
	ErrNotApproved      = errors.New("kyc not approved")
	ErrUserNotFound     = errors.New("user not found")
)
