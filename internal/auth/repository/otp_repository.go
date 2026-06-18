package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"drexa/internal/auth"
)

type otpRepository struct{ db *gorm.DB }

func NewOTPRepository(db *gorm.DB) auth.OTPRepository {
	return &otpRepository{db: db}
}

// Upsert inserts a new OTPCode, replacing any existing record for the same key.
func (r *otpRepository) Upsert(ctx context.Context, otp *auth.OTPCode) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"otp_id", "code_hash", "expires_at", "used_at", "created_at"}),
		}).
		Create(otp).Error
	if err != nil {
		return fmt.Errorf("otp_repo: upsert: %w", err)
	}
	return nil
}

func (r *otpRepository) FindByKey(ctx context.Context, key string) (*auth.OTPCode, error) {
	var o auth.OTPCode
	err := r.db.WithContext(ctx).Where(`"key" = ?`, key).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, auth.ErrOTPInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("otp_repo: find by key: %w", err)
	}
	return &o, nil
}

func (r *otpRepository) MarkUsed(ctx context.Context, otpID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&auth.OTPCode{}).
		Where("otp_id = ?", otpID).
		Update("used_at", &now).Error
}

func (r *otpRepository) DeleteExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&auth.OTPCode{}).Error
}
