package tatum

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"drexa/internal/config"
)

const baseURL = "https://api.tatum.io/v3"

type Client struct {
	cfg        config.TatumConfig
	httpClient *http.Client
}

func NewClient(cfg config.TatumConfig) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ─── Models ──────────────────────────────────────────────────────────────────

type ErrorResponse struct {
	ErrorCode  string `json:"errorCode"`
	Message    string `json:"message"`
	StatusCode int    `json:"statusCode"`
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("tatum error %s: %s (status: %d)", e.ErrorCode, e.Message, e.StatusCode)
}

type MasterWalletResponse struct {
	Xpub     string `json:"xpub"`
	Mnemonic string `json:"mnemonic"`
}

type AddressResponse struct {
	Address string `json:"address"`
}

type WebhookSubscriptionRequest struct {
	Type string `json:"type"`
	Attr struct {
		Address string `json:"address"`
		Chain   string `json:"chain"`
		URL     string `json:"url"`
	} `json:"attr"`
}

type WebhookSubscriptionResponse struct {
	ID string `json:"id"`
}

type BTCSendRequest struct {
	// Add required fields based on Tatum API for /bitcoin/transaction
	// Using mnemonic/key or fromAddress based on actual Tatum API structure
	FromAddress []BTCFromAddress `json:"fromAddress"`
	To          []BTCTo          `json:"to"`
	Fee         string           `json:"fee,omitempty"`
}

type BTCFromAddress struct {
	Address    string `json:"address"`
	PrivateKey string `json:"privateKey"`
}

type BTCTo struct {
	Address string  `json:"address"`
	Value   float64 `json:"value"` // Tatum often uses float for value in BTC
}

type ETHSendRequest struct {
	To             string `json:"to"`
	Amount         string `json:"amount"`
	FromPrivateKey string `json:"fromPrivateKey"`
}

type TransactionResponse struct {
	TxId string `json:"txId"`
}

type TxDetailResponse struct {
	Hash          string `json:"hash"`
	Confirmations int    `json:"confirmations"`
	Status        bool   `json:"status"` // specific to ETH sometimes, but let's keep it generic
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	var lastErr error
	for attempt := 0; attempt <= 3; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue // Retry on network error
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out != nil {
				if err := json.Unmarshal(respBody, out); err != nil {
					return fmt.Errorf("failed to unmarshal response: %w", err)
				}
			}
			return nil
		}

		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Message != "" {
			errResp.StatusCode = resp.StatusCode
			lastErr = &errResp
		} else {
			lastErr = &ErrorResponse{
				ErrorCode:  "UNKNOWN",
				Message:    string(respBody),
				StatusCode: resp.StatusCode,
			}
		}

		// Only retry on 429 or 5xx
		if resp.StatusCode != http.StatusTooManyRequests && (resp.StatusCode < 500 || resp.StatusCode > 599) {
			return lastErr
		}
	}

	return fmt.Errorf("max retries exceeded, last error: %w", lastErr)
}

// ─── Methods ─────────────────────────────────────────────────────────────────

func (c *Client) GenerateMasterWallet(ctx context.Context, chain string) (*MasterWalletResponse, error) {
	var path string
	if strings.HasPrefix(chain, "BTC") {
		path = "/bitcoin/wallet"
	} else if strings.HasPrefix(chain, "ETH") {
		path = "/ethereum/wallet"
	} else {
		return nil, fmt.Errorf("unsupported chain for wallet generation: %s", chain)
	}

	var res MasterWalletResponse
	err := c.doRequest(ctx, http.MethodGet, path, nil, &res) // Tatum wallet gen is usually GET
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) DeriveAddress(ctx context.Context, chain, xpub string, index int) (string, error) {
	var path string
	if strings.HasPrefix(chain, "BTC") {
		path = fmt.Sprintf("/bitcoin/address/%s/%d", xpub, index)
	} else if strings.HasPrefix(chain, "ETH") {
		path = fmt.Sprintf("/ethereum/address/%s/%d", xpub, index)
	} else {
		return "", fmt.Errorf("unsupported chain: %s", chain)
	}

	var res AddressResponse
	err := c.doRequest(ctx, http.MethodGet, path, nil, &res)
	if err != nil {
		return "", err
	}
	return res.Address, nil
}

func (c *Client) SubscribeAddressWebhook(ctx context.Context, chain, address string) (string, error) {
	req := WebhookSubscriptionRequest{
		Type: "ADDRESS_TRANSACTION",
	}
	req.Attr.Address = address
	req.Attr.Chain = chain
	req.Attr.URL = fmt.Sprintf("%s/api/v1/webhooks/tatum/deposit", c.cfg.BaseURL)

	var res WebhookSubscriptionResponse
	err := c.doRequest(ctx, http.MethodPost, "/subscription", req, &res)
	if err != nil {
		return "", err
	}
	return res.ID, nil
}

func (c *Client) SendBTCTransaction(ctx context.Context, req BTCSendRequest) (string, error) {
	var res TransactionResponse
	err := c.doRequest(ctx, http.MethodPost, "/bitcoin/transaction", req, &res)
	if err != nil {
		return "", err
	}
	return res.TxId, nil
}

func (c *Client) SendETHTransaction(ctx context.Context, req ETHSendRequest) (string, error) {
	var res TransactionResponse
	err := c.doRequest(ctx, http.MethodPost, "/ethereum/transaction", req, &res)
	if err != nil {
		return "", err
	}
	return res.TxId, nil
}

func (c *Client) GetTransactionDetail(ctx context.Context, chain, txHash string) (*TxDetailResponse, error) {
	var path string
	if strings.HasPrefix(chain, "BTC") {
		path = fmt.Sprintf("/bitcoin/transaction/%s", txHash)
	} else if strings.HasPrefix(chain, "ETH") {
		path = fmt.Sprintf("/ethereum/transaction/%s", txHash)
	} else {
		return nil, fmt.Errorf("unsupported chain: %s", chain)
	}

	var res TxDetailResponse
	err := c.doRequest(ctx, http.MethodGet, path, nil, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
