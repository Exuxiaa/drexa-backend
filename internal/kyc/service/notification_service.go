package service

import (
	"context"

	"github.com/rs/zerolog/log"
)

// MockNotificationService is used in development.
type MockNotificationService struct{}

func NewMockNotificationService() *MockNotificationService {
	return &MockNotificationService{}
}

func (m *MockNotificationService) SendKycApproved(ctx context.Context, userID, email string) error {
	log.Ctx(ctx).Info().Str("user_id", userID).Str("email", email).Msg("[mock] kyc approved email sent")
	return nil
}

func (m *MockNotificationService) SendKycRejected(ctx context.Context, userID, email, reason string) error {
	log.Ctx(ctx).Info().Str("user_id", userID).Str("email", email).Str("reason", reason).Msg("[mock] kyc rejected email sent")
	return nil
}
