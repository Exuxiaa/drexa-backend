package service

import (
	"context"
	"fmt"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"drexa/internal/auth"
	"drexa/pkg/jwt"
)

type tokenService struct {
	secret           []byte
	issuer           string
	accessExpiration time.Duration
	refreshExpiration time.Duration
}

func NewTokenService(
	secret []byte,
	issuer string,
	accessExpiration time.Duration,
	refreshExpiration time.Duration,
) auth.TokenService {
	return &tokenService{
		secret:            secret,
		issuer:            issuer,
		accessExpiration:  accessExpiration,
		refreshExpiration: refreshExpiration,
	}
}

func (s *tokenService) GenerateAccessToken(_ context.Context, user *auth.User) (string, error) {
	if user == nil {
		return "", fmt.Errorf("token_service: user must not be nil")
	}

	now := time.Now().UTC()
	claims := jwt.Claims{
		UserID:   user.UserID,
		Email:    user.Email,
		Role:     string(user.Role),
		KycLevel: user.KycLevel,
		RegisteredClaims: gojwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   user.UserID,
			IssuedAt:  gojwt.NewNumericDate(now),
			ExpiresAt: gojwt.NewNumericDate(now.Add(s.accessExpiration)),
			NotBefore: gojwt.NewNumericDate(now),
		},
	}

	return jwt.Sign(claims, s.secret)
}

func (s *tokenService) GenerateRefreshToken(_ context.Context, _ string) (string, error) {
	return jwt.NewRefreshToken()
}

func (s *tokenService) GenerateTwoFAChallengeToken(_ context.Context, userID string) (string, error) {
	now := time.Now().UTC()
	claims := jwt.Claims{
		UserID: userID,
		Scope:  "2fa_challenge",
		RegisteredClaims: gojwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID,
			IssuedAt:  gojwt.NewNumericDate(now),
			ExpiresAt: gojwt.NewNumericDate(now.Add(5 * time.Minute)),
			NotBefore: gojwt.NewNumericDate(now),
		},
	}
	return jwt.Sign(claims, s.secret)
}

func (s *tokenService) ValidateAccessToken(_ context.Context, rawToken string) (*auth.JWTClaims, error) {
	c, err := jwt.Verify(rawToken, s.secret, s.issuer)
	if err != nil {
		return nil, err
	}

	return &auth.JWTClaims{
		UserID:    c.UserID,
		Email:     c.Email,
		Role:      auth.UserRole(c.Role),
		KycLevel:  c.KycLevel,
		Scope:     c.Scope,
		ExpiresAt: c.ExpiresAt.Time,
		IssuedAt:  c.IssuedAt.Time,
	}, nil
}

func (s *tokenService) HashToken(token string) string {
	return jwt.HashToken(token)
}

func (s *tokenService) RefreshExpiration() time.Duration {
	return s.refreshExpiration
}
