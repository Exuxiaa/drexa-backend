package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"drexa/internal/kyc"
)

type kycRepository struct{ db *gorm.DB }

func New(db *gorm.DB) kyc.Repository {
	return &kycRepository{db: db}
}

func (r *kycRepository) Create(ctx context.Context, sub *kyc.Submission) error {
	if err := r.db.WithContext(ctx).Create(sub).Error; err != nil {
		return fmt.Errorf("kyc_repo: create: %w", err)
	}
	return nil
}

func (r *kycRepository) Update(ctx context.Context, sub *kyc.Submission) error {
	if err := r.db.WithContext(ctx).Save(sub).Error; err != nil {
		return fmt.Errorf("kyc_repo: update: %w", err)
	}
	return nil
}

func (r *kycRepository) UpdateStatus(ctx context.Context, submissionID string, status kyc.Status, reason, reviewedBy string) error {
	return r.db.WithContext(ctx).Model(&kyc.Submission{}).
		Where("submission_id = ?", submissionID).
		Updates(map[string]interface{}{
			"status":           status,
			"rejection_reason": reason,
			"reviewed_by":      reviewedBy,
		}).Error
}

func (r *kycRepository) FindByID(ctx context.Context, submissionID string) (*kyc.Submission, error) {
	var s kyc.Submission
	err := r.db.WithContext(ctx).Where("submission_id = ?", submissionID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kyc.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kyc_repo: find by id: %w", err)
	}
	return &s, nil
}

func (r *kycRepository) FindLatestByUserID(ctx context.Context, userID string) (*kyc.Submission, error) {
	var s kyc.Submission
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("submitted_at DESC").
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kyc.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kyc_repo: find by user: %w", err)
	}
	return &s, nil
}

func (r *kycRepository) FindByStatus(ctx context.Context, status kyc.Status) ([]kyc.Submission, error) {
	var subs []kyc.Submission
	if err := r.db.WithContext(ctx).
		Where("status = ?", status).
		Order("submitted_at ASC").
		Find(&subs).Error; err != nil {
		return nil, fmt.Errorf("kyc_repo: find by status: %w", err)
	}
	return subs, nil
}
