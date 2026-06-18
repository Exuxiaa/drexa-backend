package wallet

import (
	"errors"
	"net/http"
)

// HandleGetCryptoAddress returns (creating on first call) the user's on-chain
// deposit address and live balance for a currency. GET /wallet/crypto/address/{currency}
func HandleGetCryptoAddress(uc CryptoWalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		currency := normalizeCurrency(r.PathValue("currency"))
		if currency == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "currency is required"})
			return
		}

		asset, err := uc.GetDepositAddress(r.Context(), claims.UserID, currency)
		if err != nil {
			if errors.Is(err, ErrUnsupportedCurrency) {
				sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "currency not supported on-chain"})
				return
			}
			sendJSON(w, http.StatusBadGateway, MessageResponse{Error: "failed to get deposit address"})
			return
		}

		sendJSON(w, http.StatusOK, asset)
	}
}

// HandleGetCryptoAssets returns all supported on-chain assets with addresses and
// live balances for the user. GET /wallet/crypto/assets
func HandleGetCryptoAssets(uc CryptoWalletUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := userFromCtx(r)
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		assets, err := uc.GetAssets(r.Context(), claims.UserID)
		if err != nil {
			sendJSON(w, http.StatusBadGateway, MessageResponse{Error: "failed to load crypto assets"})
			return
		}

		sendJSON(w, http.StatusOK, assets)
	}
}
