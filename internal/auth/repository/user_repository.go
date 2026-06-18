package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"drexa/internal/auth"
)

type userRepository struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) auth.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *auth.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("user_repo: create: %w", err)
	}
	return nil
}

func (r *userRepository) Update(ctx context.Context, user *auth.User) error {
	if err := r.db.WithContext(ctx).Save(user).Error; err != nil {
		return fmt.Errorf("user_repo: update: %w", err)
	}
	return nil
}

func (r *userRepository) FindByID(ctx context.Context, userID string) (*auth.User, error) {
	var u auth.User
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, auth.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user_repo: find by id: %w", err)
	}
	return &u, nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*auth.User, error) {
	var u auth.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, auth.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user_repo: find by email: %w", err)
	}
	return &u, nil
}

func (r *userRepository) FindByPhone(ctx context.Context, phone string) (*auth.User, error) {
	var u auth.User
	err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, auth.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user_repo: find by phone: %w", err)
	}
	return &u, nil
}

func (r *userRepository) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	return r.db.WithContext(ctx).Model(&auth.User{}).
		Where("user_id = ?", userID).
		Update("password_hash", hash).Error
}

func (r *userRepository) UpdateTradingPINHash(ctx context.Context, userID, hash string) error {
	return r.db.WithContext(ctx).Model(&auth.User{}).
		Where("user_id = ?", userID).
		Update("trading_pin_hash", hash).Error
}

func (r *userRepository) UpdateTwoFA(ctx context.Context, userID, secret string, enabled bool) error {
	return r.db.WithContext(ctx).Model(&auth.User{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{"two_fa_secret": secret, "two_fa_enabled": enabled}).Error
}

func (r *userRepository) UpdateKycLevel(ctx context.Context, userID, reviewedBy string, level int) error {
	return r.db.WithContext(ctx).Model(&auth.User{}).
		Where("user_id = ?", userID).
		Update("kyc_level", level).Error
}
