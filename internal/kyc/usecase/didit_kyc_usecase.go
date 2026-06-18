package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"drexa/internal/kyc"
)

type diditKycUsecase struct {
	repo     kyc.Repository
	userSvc  kyc.UserService
	notifSvc kyc.NotificationService
	didit    kyc.DiditService
}

// NewDidit wires the Didit-backed verification usecase.
func NewDidit(
	repo kyc.Repository,
	userSvc kyc.UserService,
	notifSvc kyc.NotificationService,
	didit kyc.DiditService,
) kyc.DiditUsecase {
	return &diditKycUsecase{repo: repo, userSvc: userSvc, notifSvc: notifSvc, didit: didit}
}

func (uc *diditKycUsecase) Service() kyc.DiditService { return uc.didit }
func (uc *diditKycUsecase) Repo() kyc.Repository      { return uc.repo }

// StartVerification creates a Didit session (vendor_data = our user id) and
// persists a pending submission tracking that session. Returns the hosted url.
func (uc *diditKycUsecase) StartVerification(ctx context.Context, userID string) (*kyc.DiditSession, error) {
	if _, err := uc.userSvc.FindByID(ctx, userID); err != nil {
		return nil, kyc.ErrUserNotFound
	}

	session, err := uc.didit.CreateSession(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Document fields are collected by Didit on its hosted flow; persist empty
	// (NOT NULL columns are satisfied by ''), keyed by the Didit session.
	sub := &kyc.Submission{
		SubmissionID:    uuid.NewString(),
		UserID:          userID,
		Status:          kyc.StatusPending,
		SubmittedAt:     time.Now(),
		DiditSessionID:  session.SessionID,
		DiditSessionURL: session.URL,
		DiditStatus:     session.Status,
	}
	if err := uc.repo.Create(ctx, sub); err != nil {
		return nil, fmt.Errorf("didit_kyc: persist session: %w", err)
	}
	return session, nil
}

// HandleWebhook applies an already-verified, deduped Didit decision.
func (uc *diditKycUsecase) HandleWebhook(ctx context.Context, event *kyc.DiditWebhookEvent) error {
	// vendor_data is our internal user id; fall back to the tracked submission.
	userID := event.VendorData
	if sub, err := uc.repo.FindByDiditSessionID(ctx, event.SessionID); err == nil && userID == "" {
		userID = sub.UserID
	}

	switch event.Status {
	case kyc.DiditApproved:
		if err := uc.repo.UpdateDiditResult(ctx, event.SessionID, event.Status, kyc.StatusApproved); err != nil {
			return err
		}
		if userID != "" {
			if err := uc.userSvc.UpdateKycLevel(ctx, userID, "didit", 1); err != nil {
				return fmt.Errorf("didit_kyc: update kyc level: %w", err)
			}
			if user, err := uc.userSvc.FindByID(ctx, userID); err == nil {
				_ = uc.notifSvc.SendKycApproved(ctx, user.UserID, user.Email)
			}
		}

	case kyc.DiditDeclined:
		if err := uc.repo.UpdateDiditResult(ctx, event.SessionID, event.Status, kyc.StatusRejected); err != nil {
			return err
		}
		if userID != "" {
			if user, err := uc.userSvc.FindByID(ctx, userID); err == nil {
				_ = uc.notifSvc.SendKycRejected(ctx, user.UserID, user.Email, "identity verification declined")
			}
		}

	case kyc.DiditKycExpired:
		// Verified user's KYC aged out — downgrade and leave a trail.
		if err := uc.repo.UpdateDiditResult(ctx, event.SessionID, event.Status, kyc.StatusRejected); err != nil {
			return err
		}
		if userID != "" {
			_ = uc.userSvc.UpdateKycLevel(ctx, userID, "didit", 0)
		}

	default:
		// "Not Started" | "In Progress" | "Awaiting User" | "In Review" |
		// "Resubmitted" | "Abandoned" | "Expired" — track raw status, keep the
		// submission pending (no lifecycle advance).
		if err := uc.repo.UpdateDiditResult(ctx, event.SessionID, event.Status, ""); err != nil {
			return err
		}
	}

	log.Ctx(ctx).Info().
		Str("didit_session_id", event.SessionID).
		Str("didit_status", event.Status).
		Msg("didit_kyc: webhook applied")
	return nil
}
