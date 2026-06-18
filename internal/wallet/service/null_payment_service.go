package service

import (
	"context"
	"fmt"
	"time"

	"drexa/internal/wallet"
)

// NullPaymentService is a no-op implementation of PaymentService for local development.
// It auto-generates fake provider references without calling any real payment API.
// Replace with StripePaymentService or MidtransPaymentService for production.
type NullPaymentService struct{}

func NewNullPaymentService() wallet.PaymentService {
	return &NullPaymentService{}
}

func (s *NullPaymentService) CreatePaymentSession(
	ctx context.Context,
	depositID string,
	amount int64,
	currency wallet.CurrencyCode,
	userEmail string,
) (sessionURL, providerRef string, err error) {
	fakeRef := fmt.Sprintf("null_dep_%s_%d", depositID[:8], time.Now().Unix())
	fakeURL := fmt.Sprintf("http://localhost:3000/wallet/deposit/mock?ref=%s", fakeRef)
	return fakeURL, fakeRef, nil
}

func (s *NullPaymentService) CreatePaymentIntent(
	ctx context.Context,
	depositID string,
	amount int64,
	currency wallet.CurrencyCode,
	userEmail string,
) (clientSecret, providerRef string, err error) {
	// Mimic Stripe's "pi_xxx" / "pi_xxx_secret_yyy" shape so the frontend Stripe Elements
	// form treats the mock secret like a real one in local development.
	providerRef = fmt.Sprintf("pi_null_%s_%d", depositID[:8], time.Now().Unix())
	clientSecret = providerRef + "_secret_mock"
	return clientSecret, providerRef, nil
}

func (s *NullPaymentService) CreateDisbursement(
	ctx context.Context,
	req *wallet.DisbursementRequest,
) (providerRef string, err error) {
	fakeRef := fmt.Sprintf("null_dis_%s_%d", req.WithdrawalID[:8], time.Now().Unix())
	return fakeRef, nil
}
