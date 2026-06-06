package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"drexa/internal/auth"
)

var pinRegexp = regexp.MustCompile(`^\d{6}$`)

type authUsecase struct {
	userRepo         auth.UserRepository
	refreshTokenRepo auth.RefreshTokenRepository
	otpService       auth.OTPService
	tokenService     auth.TokenService
}

func NewAuthUsecase(
	userRepo auth.UserRepository,
	refreshTokenRepo auth.RefreshTokenRepository,
	otpService auth.OTPService,
	tokenService auth.TokenService,
) auth.AuthUsecase {
	return &authUsecase{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		otpService:       otpService,
		tokenService:     tokenService,
	}
}

// SignInWithFirebase is the single entry point for all auth flows.
// It finds the user by Firebase UID, creating them on first sign-in.
func (uc *authUsecase) SignInWithFirebase(ctx context.Context, claims *auth.FirebaseClaims) (*auth.AuthToken, error) {
	user, err := uc.userRepo.FindByFirebaseUID(ctx, claims.UID)
	if err != nil {
		if !errors.Is(err, auth.ErrUserNotFound) {
			return nil, err
		}
		user = &auth.User{
			UserID:          uuid.NewString(),
			FirebaseUID:     claims.UID,
			Email:           claims.Email,
			IsEmailVerified: claims.EmailVerified,
			LastLoginAt:     time.Now(),
		}
		if err := uc.userRepo.Create(ctx, user); err != nil {
			return nil, errors.New("failed to create user")
		}
	} else if claims.EmailVerified && !user.IsEmailVerified {
		_ = uc.userRepo.UpdateEmailVerified(ctx, user.UserID, true)
		user.IsEmailVerified = true
	}

	return uc.issueTokenPair(ctx, user)
}

func (uc *authUsecase) SendPhoneVerificationOTP(ctx context.Context, userID string) error {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return auth.ErrUserNotFound
	}
	return uc.otpService.GenerateAndSendSMS(ctx, fmt.Sprintf("otp:phone:%s", user.PhoneNumber), user.PhoneNumber)
}

func (uc *authUsecase) VerifyPhone(ctx context.Context, userID, otp string) (bool, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return false, auth.ErrUserNotFound
	}

	ok, err := uc.otpService.Verify(ctx, fmt.Sprintf("otp:phone:%s", user.PhoneNumber), otp)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	return true, uc.userRepo.UpdatePhoneVerified(ctx, userID, true)
}

func (uc *authUsecase) RefreshToken(ctx context.Context, rawToken string) (*auth.AuthToken, error) {
	hash := uc.tokenService.HashToken(rawToken)
	stored, err := uc.refreshTokenRepo.FindByTokenHash(ctx, hash)
	if err != nil {
		return nil, auth.ErrTokenInvalid
	}

	if stored.RevokedAt != nil || time.Now().After(stored.ExpiresAt) {
		return nil, auth.ErrTokenExpired
	}

	user, err := uc.userRepo.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, auth.ErrUserNotFound
	}

	_ = uc.refreshTokenRepo.Revoke(ctx, stored.TokenID)
	return uc.issueTokenPair(ctx, user)
}

func (uc *authUsecase) Logout(ctx context.Context, rawToken string) error {
	hash := uc.tokenService.HashToken(rawToken)
	stored, err := uc.refreshTokenRepo.FindByTokenHash(ctx, hash)
	if err != nil {
		return nil
	}
	return uc.refreshTokenRepo.Revoke(ctx, stored.TokenID)
}

func (uc *authUsecase) LogoutAll(ctx context.Context, userID string) error {
	return uc.refreshTokenRepo.RevokeAllByUserID(ctx, userID)
}

func (uc *authUsecase) SetTradingPin(ctx context.Context, userID, pin string) error {
	if !pinRegexp.MatchString(pin) {
		return errors.New("pin must be exactly 6 digits")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("failed to hash pin")
	}
	return uc.userRepo.UpdateTradingPinHash(ctx, userID, string(hash))
}

func (uc *authUsecase) VerifyTradingPin(ctx context.Context, userID, pin string) (bool, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return false, auth.ErrUserNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.TradingPinHash), []byte(pin)); err != nil {
		return false, nil
	}
	return true, nil
}

func (uc *authUsecase) issueTokenPair(ctx context.Context, user *auth.User) (*auth.AuthToken, error) {
	accToken, err := uc.tokenService.GenerateAccessToken(ctx, user)
	if err != nil {
		return nil, err
	}

	rawRefToken, err := uc.tokenService.GenerateRefreshToken(ctx, user.UserID)
	if err != nil {
		return nil, err
	}

	record := &auth.RefreshToken{
		TokenID:   uuid.NewString(),
		UserID:    user.UserID,
		TokenHash: uc.tokenService.HashToken(rawRefToken),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := uc.refreshTokenRepo.Create(ctx, record); err != nil {
		return nil, err
	}

	if err := uc.userRepo.UpdateLastLoginAt(ctx, user.UserID); err != nil {
		log.Printf("issueTokenPair: failed to update last_login_at for user %s: %v", user.UserID, err)
	}

	return &auth.AuthToken{
		AccessToken:  accToken,
		RefreshToken: rawRefToken,
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(15 * time.Minute),
	}, nil
}
