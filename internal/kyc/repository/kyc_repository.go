package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

func (r *kycRepository) FindByDiditSessionID(ctx context.Context, sessionID string) (*kyc.Submission, error) {
	var s kyc.Submission
	err := r.db.WithContext(ctx).Where("didit_session_id = ?", sessionID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kyc.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kyc_repo: find by didit session: %w", err)
	}
	return &s, nil
}

func (r *kycRepository) UpdateDiditResult(ctx context.Context, sessionID, diditStatus string, status kyc.Status) error {
	updates := map[string]interface{}{"didit_status": diditStatus}
	// Only advance the submission's own lifecycle on terminal decisions.
	if status != "" {
		updates["status"] = status
	}
	return r.db.WithContext(ctx).Model(&kyc.Submission{}).
		Where("didit_session_id = ?", sessionID).
		Updates(updates).Error
}

func (r *kycRepository) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&kyc.ProcessedEvent{}).
		Where("event_id = ?", eventID).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("kyc_repo: is event processed: %w", err)
	}
	return count > 0, nil
}

func (r *kycRepository) MarkEventProcessed(ctx context.Context, eventID string) error {
	// ON CONFLICT DO NOTHING — concurrent deliveries of the same event_id are safe.
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&kyc.ProcessedEvent{EventID: eventID}).Error
	if err != nil {
		return fmt.Errorf("kyc_repo: mark event processed: %w", err)
	}
	return nil
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
