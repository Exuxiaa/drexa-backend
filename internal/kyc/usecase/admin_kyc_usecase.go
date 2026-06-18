package usecase

import (
	"context"
	"fmt"

	"drexa/internal/kyc"
)

type adminKycUsecase struct {
	repo     kyc.Repository
	userSvc  kyc.UserService
	notifSvc kyc.NotificationService
}

func NewAdmin(repo kyc.Repository, userSvc kyc.UserService, notifSvc kyc.NotificationService) kyc.AdminUsecase {
	return &adminKycUsecase{repo: repo, userSvc: userSvc, notifSvc: notifSvc}
}

func (uc *adminKycUsecase) ListByStatus(ctx context.Context, status kyc.Status) ([]kyc.Submission, error) {
	return uc.repo.FindByStatus(ctx, status)
}

func (uc *adminKycUsecase) GetByID(ctx context.Context, submissionID string) (*kyc.Submission, error) {
	return uc.repo.FindByID(ctx, submissionID)
}

func (uc *adminKycUsecase) Approve(ctx context.Context, submissionID, reviewedBy string) error {
	sub, err := uc.repo.FindByID(ctx, submissionID)
	if err != nil {
		return kyc.ErrNotFound
	}

	if err := uc.repo.UpdateStatus(ctx, submissionID, kyc.StatusApproved, "", reviewedBy); err != nil {
		return fmt.Errorf("admin_kyc: approve: %w", err)
	}

	if err := uc.userSvc.UpdateKycLevel(ctx, sub.UserID, reviewedBy, 1); err != nil {
		return fmt.Errorf("admin_kyc: update kyc level: %w", err)
	}

	user, err := uc.userSvc.FindByID(ctx, sub.UserID)
	if err == nil {
		_ = uc.notifSvc.SendKycApproved(ctx, user.UserID, user.Email)
	}

	return nil
}

func (uc *adminKycUsecase) Reject(ctx context.Context, submissionID, reviewedBy, reason string) error {
	sub, err := uc.repo.FindByID(ctx, submissionID)
	if err != nil {
		return kyc.ErrNotFound
	}

	if err := uc.repo.UpdateStatus(ctx, submissionID, kyc.StatusRejected, reason, reviewedBy); err != nil {
		return fmt.Errorf("admin_kyc: reject: %w", err)
	}

	user, err := uc.userSvc.FindByID(ctx, sub.UserID)
	if err == nil {
		_ = uc.notifSvc.SendKycRejected(ctx, user.UserID, user.Email, reason)
	}

	return nil
}
