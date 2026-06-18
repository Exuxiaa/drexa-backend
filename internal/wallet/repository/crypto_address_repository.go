package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"drexa/internal/wallet"
)

type cryptoAddressRepository struct {
	db *gorm.DB
}

func NewCryptoAddressRepository(db *gorm.DB) wallet.CryptoAddressRepository {
	return &cryptoAddressRepository{db: db}
}

func (r *cryptoAddressRepository) Create(ctx context.Context, addr *wallet.CryptoAddress) error {
	return dbFromContext(ctx, r.db).Create(addr).Error
}

func (r *cryptoAddressRepository) FindByUserAndCurrency(ctx context.Context, userID string, currency wallet.CurrencyCode) (*wallet.CryptoAddress, error) {
	var addr wallet.CryptoAddress
	err := dbFromContext(ctx, r.db).
		Where("user_id = ? AND currency = ?", userID, currency).
		First(&addr).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, wallet.ErrCryptoAddressNotFound
		}
		return nil, err
	}
	return &addr, nil
}
