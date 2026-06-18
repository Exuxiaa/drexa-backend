package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"drexa/internal/wallet"
	"drexa/pkg/config"
)

// TatumService talks to the Tatum v3 API for HD wallet generation, address
// derivation, and on-chain balance lookups. The configured API key selects the
// network (a testnet key returns testnet addresses/balances).
type TatumService struct {
	cfg  config.TatumConfig
	http *http.Client
}

func NewTatumService(cfg config.TatumConfig, baseURL string) *TatumService {
	return &TatumService{
		cfg:  cfg,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

var _ wallet.CryptoProvider = (*TatumService)(nil)

func (s *TatumService) doRequest(ctx context.Context, method, path string, body interface{}, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, "https://api.tatum.io"+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", s.cfg.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("tatum: request %s: %w", path, err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var errBody struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(res.Body).Decode(&errBody)
		return fmt.Errorf("tatum: %s returned %d: %s", path, res.StatusCode, errBody.Message)
	}

	return json.NewDecoder(res.Body).Decode(out)
}

// GenerateWallet creates a new HD wallet for a chain and returns its xpub.
func (s *TatumService) GenerateWallet(ctx context.Context, chain string) (string, error) {
	var resp struct {
		Xpub     string `json:"xpub"`
		Mnemonic string `json:"mnemonic"`
	}
	if err := s.doRequest(ctx, http.MethodGet, fmt.Sprintf("/v3/%s/wallet", chain), nil, &resp); err != nil {
		return "", err
	}
	// Note: mnemonic is intentionally discarded — these addresses are deposit-only.
	// Spending (withdrawals) would require securely persisting the mnemonic (KMS).
	return resp.Xpub, nil
}

// DeriveAddress derives the receiving address for an xpub at the given index.
func (s *TatumService) DeriveAddress(ctx context.Context, chain, xpub string, index int) (string, error) {
	var resp struct {
		Address string `json:"address"`
	}
	if err := s.doRequest(ctx, http.MethodGet, fmt.Sprintf("/v3/%s/address/%s/%d", chain, xpub, index), nil, &resp); err != nil {
		return "", err
	}
	return resp.Address, nil
}

// GetBalance returns the address balance in the coin's main unit as a decimal string.
func (s *TatumService) GetBalance(ctx context.Context, chain, address string) (string, error) {
	switch chain {
	case "bitcoin":
		// BTC returns incoming/outgoing totals; net balance = incoming - outgoing.
		var resp struct {
			Incoming string `json:"incoming"`
			Outgoing string `json:"outgoing"`
		}
		if err := s.doRequest(ctx, http.MethodGet, fmt.Sprintf("/v3/%s/address/balance/%s", chain, address), nil, &resp); err != nil {
			return "", err
		}
		in, _ := strconv.ParseFloat(resp.Incoming, 64)
		out, _ := strconv.ParseFloat(resp.Outgoing, 64)
		return strconv.FormatFloat(in-out, 'f', -1, 64), nil

	default:
		// ETH (and EVM chains) expose a single balance field in the main unit.
		var resp struct {
			Balance string `json:"balance"`
		}
		if err := s.doRequest(ctx, http.MethodGet, fmt.Sprintf("/v3/%s/account/balance/%s", chain, address), nil, &resp); err != nil {
			return "", err
		}
		if resp.Balance == "" {
			return "0", nil
		}
		return resp.Balance, nil
	}
}

// SendTransaction is a stub for crypto withdrawals.
// Real implementation requires securely managing the private key (e.g. via KMS) to sign transactions.
func (s *TatumService) SendTransaction(ctx context.Context, chain string, amount string, toAddress string) (string, error) {
	if chain == "bitcoin" {
		amtFloat, err := strconv.ParseFloat(amount, 64)
		if err != nil {
			return "", fmt.Errorf("invalid btc amount: %w", err)
		}
		
		req := map[string]interface{}{
			"fromAddress": []map[string]interface{}{
				{
					"address":    s.cfg.BTCAddress,
					"privateKey": s.cfg.BTCPrivateKey,
				},
			},
			"to": []map[string]interface{}{
				{
					"address": toAddress,
					"value":   amtFloat,
				},
			},
		}

		var resp struct {
			TxId string `json:"txId"`
		}
		if err := s.doRequest(ctx, http.MethodPost, "/v3/bitcoin/transaction", req, &resp); err != nil {
			return "", err
		}
		return resp.TxId, nil

	} else if chain == "ethereum" {
		req := map[string]interface{}{
			"to":             toAddress,
			"amount":         amount,
			"fromPrivateKey": s.cfg.ETHPrivateKey,
		}
		var resp struct {
			TxId string `json:"txId"`
		}
		if err := s.doRequest(ctx, http.MethodPost, "/v3/ethereum/transaction", req, &resp); err != nil {
			return "", err
		}
		return resp.TxId, nil
	}

	return "", fmt.Errorf("unsupported chain for sending transactions: %s", chain)
}
