package service

import (
	"context"
	"fmt"

	"drexa/internal/auth"
)

type sendgridNotificationService struct {
	email  EmailSender
	appURL string
}

// NewSendGridNotificationService returns a NotificationService backed by SendGrid.
// Pass the same EmailSender used for OTP to reuse the same client.
func NewSendGridNotificationService(email EmailSender, appURL string) auth.NotificationService {
	return &sendgridNotificationService{email: email, appURL: appURL}
}

func (s *sendgridNotificationService) SendPasswordChanged(ctx context.Context, _ string, email string) error {
	body := "Your Drexa account password was changed. If you did not do this, please contact support immediately."
	return s.email.SendEmail(ctx, email, "Password Changed — Drexa", body)
}

func (s *sendgridNotificationService) SendNewLogin(ctx context.Context, _ string, email, userAgent, ipAddress string) error {
	body := fmt.Sprintf("A new login was detected on your Drexa account.\n\nDevice: %s\nIP: %s\n\nIf this wasn't you, please change your password immediately.", userAgent, ipAddress)
	return s.email.SendEmail(ctx, email, "New Login Detected — Drexa", body)
}

func (s *sendgridNotificationService) SendPasswordReset(ctx context.Context, _ string, email, rawToken string) error {
	link := fmt.Sprintf("%s/auth/reset-password?token=%s", s.appURL, rawToken)
	body := fmt.Sprintf("Click the link below to reset your Drexa password. This link expires in 1 hour.\n\n%s\n\nIf you did not request this, ignore this email.", link)
	return s.email.SendEmail(ctx, email, "Reset Your Password — Drexa", body)
}
