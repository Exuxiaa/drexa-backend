package jwt

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the payload embedded inside every access token.
type Claims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	KycLevel int    `json:"kyc_level"`
	// Scope is non-empty for restricted tokens (e.g. "2fa_challenge").
	// Normal access tokens always have Scope == "".
	Scope string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

// Sign issues a signed HS256 JWT from claims.
func Sign(claims Claims, secret []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("jwt: sign: %w", err)
	}
	return signed, nil
}

// Verify parses and validates a JWT, returning the embedded claims.
func Verify(raw string, secret []byte, issuer string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(
		raw,
		&Claims{},
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("jwt: unexpected signing method: %v", t.Header["alg"])
			}
			return secret, nil
		},
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("jwt: verify: %w", err)
	}
	c, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("jwt: invalid claims")
	}
	return c, nil
}

// NewRefreshToken generates a 32-byte cryptographically random opaque token.
func NewRefreshToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("jwt: refresh token entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// HashToken returns the hex-encoded SHA-256 digest of token.
// Store only the hash in the DB; send the raw token over the wire.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// AccessExpiry returns the absolute expiry time for an access token.
func AccessExpiry(ttl time.Duration) time.Time {
	return time.Now().UTC().Add(ttl)
}
