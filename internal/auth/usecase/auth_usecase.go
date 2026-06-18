package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"drexa/internal/auth"
	"drexa/pkg/password"
)

var (
	pinRegexp = regexp.MustCompile(`^\d{6}$`)
)

type authUsecase struct {
	userRepo    auth.UserRepository
	tokenRepo   auth.RefreshTokenRepository
	otpService  auth.OTPService
	tokenSvc    auth.TokenService
}

func NewAuthUsecase(
	userRepo auth.UserRepository,
	tokenRepo auth.RefreshTokenRepository,
	otpService auth.OTPService,
	tokenSvc auth.TokenService,
) auth.AuthUsecase {
	return &authUsecase{
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
		otpService: otpService,
		tokenSvc:   tokenSvc,
	}
}

func (uc *authUsecase) Register(ctx context.Context, email, phone, pw string) (*auth.User, error) {
	if _, err := uc.userRepo.FindByEmail(ctx, email); err == nil {
		return nil, auth.ErrEmailAlreadyExists
	}
	if _, err := uc.userRepo.FindByPhone(ctx, phone); err == nil {
		return nil, auth.ErrPhoneAlreadyExists
	}

	hash, err := password.Hash(pw)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	user := &auth.User{
		UserID:       uuid.NewString(),
		Email:        email,
		Phone:        phone,
		PasswordHash: hash,
		Role:         auth.RoleUser,
		KycLevel:     0,
	}

	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("register: create user: %w", err)
	}

	return user, nil
}

func (uc *authUsecase) Login(ctx context.Context, email, pw string) (*auth.AuthToken, error) {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, auth.ErrInvalidCredentials
	}

	if err := password.Check(pw, user.PasswordHash); err != nil {
		return nil, auth.ErrInvalidCredentials
	}

	if user.TwoFAEnabled {
		challengeToken, err := uc.tokenSvc.GenerateTwoFAChallengeToken(ctx, user.UserID)
		if err != nil {
			return nil, fmt.Errorf("login: generate 2fa challenge: %w", err)
		}
		return &auth.AuthToken{RequiresTwoFA: true, ChallengeToken: challengeToken}, nil
	}

	return uc.issueTokenPair(ctx, user)
}

func (uc *authUsecase) RefreshToken(ctx context.Context, rawToken string) (*auth.AuthToken, error) {
	hash := uc.tokenSvc.HashToken(rawToken)
	stored, err := uc.tokenRepo.FindByTokenHash(ctx, hash)
	if err != nil {
		return nil, auth.ErrTokenInvalid
	}

	if stored.RevokedAt != nil || time.Now().After(stored.ExpiredAt) {
		return nil, auth.ErrTokenExpired
	}

	user, err := uc.userRepo.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, auth.ErrUserNotFound
	}

	newToken, err := uc.issueTokenPair(ctx, user)
	if err != nil {
		return nil, err
	}
	_ = uc.tokenRepo.Revoke(ctx, stored.TokenID)
	return newToken, nil
}

func (uc *authUsecase) Logout(ctx context.Context, rawToken string) error {
	hash := uc.tokenSvc.HashToken(rawToken)
	stored, err := uc.tokenRepo.FindByTokenHash(ctx, hash)
	if err != nil {
		return nil // already invalid — treat as success
	}
	return uc.tokenRepo.Revoke(ctx, stored.TokenID)
}

func (uc *authUsecase) LogoutAll(ctx context.Context, userID string) error {
	return uc.tokenRepo.RevokeAllByUserID(ctx, userID)
}

func (uc *authUsecase) ChangePassword(ctx context.Context, userID, oldPW, newPW string) error {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return auth.ErrUserNotFound
	}

	if err := password.Check(oldPW, user.PasswordHash); err != nil {
		return auth.ErrInvalidCredentials
	}

	hash, err := password.Hash(newPW)
	if err != nil {
		return fmt.Errorf("change_password: hash: %w", err)
	}

	return uc.userRepo.UpdatePasswordHash(ctx, userID, hash)
}

func (uc *authUsecase) SendPhoneOTP(ctx context.Context, userID string) error {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return auth.ErrUserNotFound
	}
	key := fmt.Sprintf("otp:phone:%s", user.Phone)
	return uc.otpService.GenerateAndSendSMS(ctx, key, user.Phone)
}

func (uc *authUsecase) VerifyPhoneOTP(ctx context.Context, userID, otp string) error {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return auth.ErrUserNotFound
	}

	key := fmt.Sprintf("otp:phone:%s", user.Phone)
	ok, err := uc.otpService.Verify(ctx, key, otp)
	if err != nil || !ok {
		return auth.ErrOTPInvalid
	}
	return nil
}

func (uc *authUsecase) SetTradingPIN(ctx context.Context, userID, pin string) error {
	if !pinRegexp.MatchString(pin) {
		return errors.New("PIN must be exactly 6 digits")
	}
	hash, err := password.Hash(pin)
	if err != nil {
		return fmt.Errorf("set_pin: hash: %w", err)
	}
	return uc.userRepo.UpdateTradingPINHash(ctx, userID, hash)
}

func (uc *authUsecase) VerifyTradingPIN(ctx context.Context, userID, pin string) (bool, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return false, auth.ErrUserNotFound
	}
	if err := password.Check(pin, user.TradingPINHash); err != nil {
		return false, nil
	}
	return true, nil
}

func (uc *authUsecase) InitiateTwoFA(ctx context.Context, userID string) (*auth.TwoFASetup, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, auth.ErrUserNotFound
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Drexa",
		AccountName: user.Email,
	})
	if err != nil {
		return nil, fmt.Errorf("initiate_2fa: generate key: %w", err)
	}

	// Store secret without enabling yet; user must confirm with a valid code.
	if err := uc.userRepo.UpdateTwoFA(ctx, userID, key.Secret(), false); err != nil {
		return nil, fmt.Errorf("initiate_2fa: store secret: %w", err)
	}

	return &auth.TwoFASetup{
		Secret:    key.Secret(),
		QRCodeURL: key.URL(),
	}, nil
}

func (uc *authUsecase) ConfirmTwoFA(ctx context.Context, userID, code string) error {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return auth.ErrUserNotFound
	}
	if user.TwoFASecret == "" {
		return errors.New("2FA not initiated — call setup first")
	}
	if !totp.Validate(code, user.TwoFASecret) {
		return errors.New("invalid TOTP code")
	}
	return uc.userRepo.UpdateTwoFA(ctx, userID, user.TwoFASecret, true)
}

func (uc *authUsecase) DisableTwoFA(ctx context.Context, userID, code string) error {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return auth.ErrUserNotFound
	}
	if !user.TwoFAEnabled {
		return errors.New("2FA is not enabled")
	}
	if !totp.Validate(code, user.TwoFASecret) {
		return errors.New("invalid TOTP code")
	}
	return uc.userRepo.UpdateTwoFA(ctx, userID, "", false)
}

func (uc *authUsecase) VerifyTwoFA(ctx context.Context, userID, code string) (*auth.AuthToken, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, auth.ErrUserNotFound
	}
	if !user.TwoFAEnabled {
		return nil, errors.New("2FA not enabled for this account")
	}
	if !totp.Validate(code, user.TwoFASecret) {
		return nil, errors.New("invalid TOTP code")
	}
	return uc.issueTokenPair(ctx, user)
}

func (uc *authUsecase) issueTokenPair(ctx context.Context, user *auth.User) (*auth.AuthToken, error) {
	accToken, err := uc.tokenSvc.GenerateAccessToken(ctx, user)
	if err != nil {
		return nil, err
	}

	rawRefToken, err := uc.tokenSvc.GenerateRefreshToken(ctx, user.UserID)
	if err != nil {
		return nil, err
	}

	record := &auth.RefreshToken{
		TokenID:   uuid.NewString(),
		UserID:    user.UserID,
		TokenHash: uc.tokenSvc.HashToken(rawRefToken),
		ExpiredAt: time.Now().Add(uc.tokenSvc.RefreshExpiration()),
	}
	if err := uc.tokenRepo.Create(ctx, record); err != nil {
		return nil, fmt.Errorf("issue_token_pair: store refresh token: %w", err)
	}

	return &auth.AuthToken{
		AccessToken:  accToken,
		RefreshToken: rawRefToken,
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(15 * time.Minute),
	}, nil
}
