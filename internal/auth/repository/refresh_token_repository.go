package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"drexa/internal/auth"
)

type refreshTokenRepository struct{ db *gorm.DB }

func NewRefreshTokenRepository(db *gorm.DB) auth.RefreshTokenRepository {
	return &refreshTokenRepository{db: db}
}

func (r *refreshTokenRepository) Create(ctx context.Context, token *auth.RefreshToken) error {
	if err := r.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("refresh_token_repo: create: %w", err)
	}
	return nil
}

func (r *refreshTokenRepository) Revoke(ctx context.Context, tokenID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&auth.RefreshToken{}).
		Where("token_id = ?", tokenID).
		Update("revoked_at", &now).Error
}

func (r *refreshTokenRepository) RevokeAllByUserID(ctx context.Context, userID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&auth.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", &now).Error
}

func (r *refreshTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*auth.RefreshToken, error) {
	var t auth.RefreshToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, auth.ErrTokenInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("refresh_token_repo: find by hash: %w", err)
	}
	return &t, nil
}

func (r *refreshTokenRepository) FindActiveByUserID(ctx context.Context, userID string) ([]auth.RefreshToken, error) {
	var tokens []auth.RefreshToken
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL AND expired_at > ?", userID, time.Now()).
		Find(&tokens).Error
	if err != nil {
		return nil, fmt.Errorf("refresh_token_repo: find active: %w", err)
	}
	return tokens, nil
}

func (r *refreshTokenRepository) DeleteExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Where("expired_at < ?", time.Now()).
		Delete(&auth.RefreshToken{}).Error
}
