package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"drexa/internal/auth"
)

const otpTTL = 10 * time.Minute

type otpService struct {
	otpRepo  auth.OTPRepository
	emailSvc EmailSender
	smsSvc   SMSSender
}

// NewOTPService creates a PostgreSQL-backed OTP service.
func NewOTPService(otpRepo auth.OTPRepository, emailSvc EmailSender, smsSvc SMSSender) auth.OTPService {
	return &otpService{otpRepo: otpRepo, emailSvc: emailSvc, smsSvc: smsSvc}
}

func (s *otpService) GenerateAndSendSMS(ctx context.Context, key, phone string) error {
	code, err := s.store(ctx, key)
	if err != nil {
		return err
	}
	return s.smsSvc.SendSMS(ctx, phone, fmt.Sprintf("Your Drexa verification code: %s", code))
}

func (s *otpService) GenerateAndSendEmail(ctx context.Context, key, email string) error {
	code, err := s.store(ctx, key)
	if err != nil {
		return err
	}
	return s.emailSvc.SendEmail(ctx, email, "Your Drexa Verification Code",
		fmt.Sprintf("Your code: %s\nThis code expires in 10 minutes.", code))
}

func (s *otpService) Verify(ctx context.Context, key, code string) (bool, error) {
	otp, err := s.otpRepo.FindByKey(ctx, key)
	if err != nil {
		return false, nil // treat not-found as invalid, not error
	}

	if otp.UsedAt != nil || time.Now().After(otp.ExpiresAt) {
		return false, nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(otp.CodeHash), []byte(code)); err != nil {
		return false, nil
	}

	_ = s.otpRepo.MarkUsed(ctx, otp.OTPID)
	return true, nil
}

func (s *otpService) store(ctx context.Context, key string) (string, error) {
	code, err := generateCode()
	if err != nil {
		return "", fmt.Errorf("otp: generate code: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.MinCost)
	if err != nil {
		return "", fmt.Errorf("otp: hash code: %w", err)
	}

	otp := &auth.OTPCode{
		OTPID:     uuid.NewString(),
		Key:       key,
		CodeHash:  string(hash),
		ExpiresAt: time.Now().Add(otpTTL),
	}

	if err := s.otpRepo.Upsert(ctx, otp); err != nil {
		return "", fmt.Errorf("otp: store: %w", err)
	}

	return code, nil
}

// generateCode returns a cryptographically random 6-digit string.
func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
