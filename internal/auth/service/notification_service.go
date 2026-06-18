package service

import (
	"context"

	"github.com/rs/zerolog/log"

	"drexa/internal/auth"
)

// mockNotificationService logs all notifications instead of calling a real provider.
// For development only — replace with NewSendGridNotificationService in production.
type mockNotificationService struct{}

func NewMockNotificationService() auth.NotificationService {
	return &mockNotificationService{}
}

func (s *mockNotificationService) SendPasswordChanged(_ context.Context, userID, email string) error {
	log.Info().Str("user_id", userID).Str("email", email).Msg("[mock] password_changed")
	return nil
}

func (s *mockNotificationService) SendNewLogin(_ context.Context, userID, email, userAgent, ip string) error {
	log.Info().Str("user_id", userID).Str("email", email).Str("ip", ip).Msg("[mock] new_login")
	return nil
}
