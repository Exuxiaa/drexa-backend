package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"drexa/internal/kyc"
)

type kycUsecase struct {
	repo    kyc.Repository
	userSvc kyc.UserService
}

func New(repo kyc.Repository, userSvc kyc.UserService) kyc.Usecase {
	return &kycUsecase{repo: repo, userSvc: userSvc}
}

func (uc *kycUsecase) Submit(ctx context.Context, userID string, sub *kyc.Submission) error {
	if _, err := uc.userSvc.FindByID(ctx, userID); err != nil {
		return kyc.ErrUserNotFound
	}

	existing, err := uc.repo.FindLatestByUserID(ctx, userID)
	if err == nil && existing.Status == kyc.StatusPending {
		return kyc.ErrAlreadySubmitted
	}

	sub.SubmissionID = uuid.NewString()
	sub.UserID = userID
	sub.Status = kyc.StatusPending
	sub.SubmittedAt = time.Now()

	if err := uc.repo.Create(ctx, sub); err != nil {
		return errors.New("failed to submit KYC")
	}
	return nil
}

func (uc *kycUsecase) GetByUserID(ctx context.Context, userID string) (*kyc.Submission, error) {
	return uc.repo.FindLatestByUserID(ctx, userID)
}

func (uc *kycUsecase) IsApproved(ctx context.Context, userID string) (bool, error) {
	sub, err := uc.repo.FindLatestByUserID(ctx, userID)
	if err != nil {
		return false, kyc.ErrNotFound
	}
	return sub.Status == kyc.StatusApproved, nil
}
