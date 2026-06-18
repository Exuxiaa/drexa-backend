package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"drexa/internal/wallet"
)

// chainInfo maps a domain currency to its blockchain and a human-readable network label.
type chainInfo struct {
	chain   string
	mainnet string
	testnet string
}

func chainFor(c wallet.CurrencyCode, testnet bool) (chainInfo, bool) {
	table := map[wallet.CurrencyCode]chainInfo{
		wallet.CurrencyBTC: {chain: "bitcoin", mainnet: "Bitcoin", testnet: "Bitcoin testnet"},
		wallet.CurrencyETH: {chain: "ethereum", mainnet: "Ethereum", testnet: "Ethereum testnet (Sepolia)"},
	}
	info, ok := table[c]
	return info, ok
}

// supportedCryptoCurrencies lists currencies the crypto provider can serve addresses for.
var supportedCryptoCurrencies = []wallet.CurrencyCode{wallet.CurrencyBTC, wallet.CurrencyETH}

type cryptoWalletUsecase struct {
	addrRepo wallet.CryptoAddressRepository
	provider wallet.CryptoProvider
	testnet  bool
}

func NewCryptoWalletUsecase(
	addrRepo wallet.CryptoAddressRepository,
	provider wallet.CryptoProvider,
	testnet bool,
) wallet.CryptoWalletUsecase {
	return &cryptoWalletUsecase{addrRepo: addrRepo, provider: provider, testnet: testnet}
}

// getOrCreateAddress returns the persisted deposit address for a user+currency,
// generating a new HD wallet and deriving index 0 on first use.
func (uc *cryptoWalletUsecase) getOrCreateAddress(ctx context.Context, userID string, currency wallet.CurrencyCode) (*wallet.CryptoAddress, chainInfo, error) {
	info, ok := chainFor(currency, uc.testnet)
	if !ok {
		return nil, info, wallet.ErrUnsupportedCurrency
	}

	existing, err := uc.addrRepo.FindByUserAndCurrency(ctx, userID, currency)
	if err == nil {
		return existing, info, nil
	}
	if !errors.Is(err, wallet.ErrCryptoAddressNotFound) {
		return nil, info, err
	}

	// First time for this user+currency — generate a wallet and derive the address.
	xpub, err := uc.provider.GenerateWallet(ctx, info.chain)
	if err != nil {
		return nil, info, fmt.Errorf("generate wallet: %w", err)
	}
	address, err := uc.provider.DeriveAddress(ctx, info.chain, xpub, 0)
	if err != nil {
		return nil, info, fmt.Errorf("derive address: %w", err)
	}

	rec := &wallet.CryptoAddress{
		ID:              uuid.NewString(),
		UserID:          userID,
		Currency:        currency,
		Chain:           info.chain,
		Address:         address,
		Xpub:            xpub,
		DerivationIndex: 0,
	}
	if err := uc.addrRepo.Create(ctx, rec); err != nil {
		return nil, info, fmt.Errorf("save address: %w", err)
	}
	return rec, info, nil
}

func (uc *cryptoWalletUsecase) networkLabel(info chainInfo) string {
	if uc.testnet {
		return info.testnet
	}
	return info.mainnet
}

func (uc *cryptoWalletUsecase) GetDepositAddress(ctx context.Context, userID string, currency wallet.CurrencyCode) (*wallet.CryptoAsset, error) {
	rec, info, err := uc.getOrCreateAddress(ctx, userID, currency)
	if err != nil {
		return nil, err
	}

	// Best-effort live balance; never fail the address lookup over a balance hiccup.
	balance, err := uc.provider.GetBalance(ctx, info.chain, rec.Address)
	if err != nil {
		balance = "0"
	}

	return &wallet.CryptoAsset{
		Currency: currency,
		Chain:    info.chain,
		Network:  uc.networkLabel(info),
		Address:  rec.Address,
		Balance:  balance,
	}, nil
}

func (uc *cryptoWalletUsecase) GetAssets(ctx context.Context, userID string) ([]wallet.CryptoAsset, error) {
	assets := make([]wallet.CryptoAsset, 0, len(supportedCryptoCurrencies))
	for _, currency := range supportedCryptoCurrencies {
		asset, err := uc.GetDepositAddress(ctx, userID, currency)
		if err != nil {
			return nil, err
		}
		assets = append(assets, *asset)
	}
	return assets, nil
}
