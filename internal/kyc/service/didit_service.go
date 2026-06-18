package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"drexa/internal/kyc"
)

const (
	diditBaseURL        = "https://verification.didit.me"
	diditSessionPath    = "/v3/session/"
	webhookFreshnessSec = 300
)

// diditService talks to the Didit Verification API and verifies inbound webhooks.
// The x-api-key never leaves this server-side type.
type diditService struct {
	apiKey        string
	webhookSecret string
	workflowID    string
	callbackURL   string
	baseURL       string
	httpClient    *http.Client
}

// NewDiditService builds a Didit provider. callbackURL is where Didit returns
// the user after the hosted flow (your frontend route).
func NewDiditService(apiKey, webhookSecret, workflowID, callbackURL string) kyc.DiditService {
	return &diditService{
		apiKey:        apiKey,
		webhookSecret: webhookSecret,
		workflowID:    workflowID,
		callbackURL:   callbackURL,
		baseURL:       diditBaseURL,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
	}
}

// CreateSession POSTs the workflow_id + vendor_data to Didit and returns the
// hosted verification url + session id.
func (s *diditService) CreateSession(ctx context.Context, vendorData string) (*kyc.DiditSession, error) {
	reqBody := map[string]string{
		"workflow_id": s.workflowID,
		"vendor_data": vendorData,
	}
	if s.callbackURL != "" {
		reqBody["callback"] = s.callbackURL
	}

	buf, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("didit: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+diditSessionPath, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("didit: build request: %w", err)
	}
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("didit: create session: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		// 403 => missing/invalid/revoked x-api-key. Don't leak the key or full body upstream.
		return nil, fmt.Errorf("didit: create session status %d: %s", resp.StatusCode, string(body))
	}

	var session kyc.DiditSession
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("didit: decode session: %w", err)
	}
	if session.URL == "" || session.SessionID == "" {
		return nil, errors.New("didit: session response missing url or session_id")
	}
	return &session, nil
}

// VerifyWebhook enforces timestamp freshness and a constant-time HMAC-SHA256
// match against X-Signature-V2 over the canonicalised body.
func (s *diditService) VerifyWebhook(payload []byte, signatureV2 string, timestamp int64) error {
	// 1. Freshness — reject anything older/newer than 300s (replay protection).
	if timestamp == 0 || abs(time.Now().Unix()-timestamp) > webhookFreshnessSec {
		return errors.New("didit: stale webhook timestamp")
	}

	// 2. Canonicalise: shortenFloats -> sortKeys -> JSON with unescaped Unicode.
	//    Go sorts map keys on marshal (sortKeys) and renders whole-number floats
	//    without a fractional part (shortenFloats); SetEscapeHTML(false) matches
	//    JS JSON.stringify's unescaped output.
	var parsed interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return fmt.Errorf("didit: parse webhook body: %w", err)
	}
	var canonical bytes.Buffer
	enc := json.NewEncoder(&canonical)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(parsed); err != nil {
		return fmt.Errorf("didit: canonicalise webhook body: %w", err)
	}
	canonicalBytes := bytes.TrimRight(canonical.Bytes(), "\n") // Encode appends a newline

	// 3. Constant-time HMAC-SHA256 compare against X-Signature-V2.
	mac := hmac.New(sha256.New, []byte(s.webhookSecret))
	mac.Write(canonicalBytes)
	expected := hex.EncodeToString(mac.Sum(nil))

	if len(signatureV2) != len(expected) ||
		!hmac.Equal([]byte(expected), []byte(signatureV2)) {
		return errors.New("didit: webhook signature mismatch")
	}
	return nil
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
