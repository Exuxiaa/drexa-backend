package service

import (
	"context"
	"fmt"
	"strings"

	"drexa/internal/wallet"

	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/checkout/session"
	"github.com/stripe/stripe-go/v78/paymentintent"
)

type StripePaymentService struct {
	secretKey string
	appURL    string
}

func NewStripePaymentService(secretKey, appURL string) wallet.PaymentService {
	stripe.Key = secretKey
	return &StripePaymentService{
		secretKey: secretKey,
		appURL:    appURL,
	}
}

func (s *StripePaymentService) CreatePaymentSession(
	ctx context.Context,
	depositID string,
	amount int64,
	currency wallet.CurrencyCode,
	userEmail string,
) (sessionURL, providerRef string, err error) {
	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(strings.ToLower(string(currency))),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("Deposit to Wallet"),
					},
					UnitAmount: stripe.Int64(amount),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(fmt.Sprintf("%s/wallet/deposit/success?session_id={CHECKOUT_SESSION_ID}", s.appURL)),
		CancelURL:  stripe.String(fmt.Sprintf("%s/wallet/deposit/cancel", s.appURL)),
		ClientReferenceID: stripe.String(depositID),
	}
	if userEmail != "" {
		params.CustomerEmail = stripe.String(userEmail)
	}
	
	params.Context = ctx

	sess, err := session.New(params)
	if err != nil {
		return "", "", fmt.Errorf("failed to create stripe checkout session: %w", err)
	}

	return sess.URL, sess.ID, nil
}

func (s *StripePaymentService) CreatePaymentIntent(
	ctx context.Context,
	depositID string,
	amount int64,
	currency wallet.CurrencyCode,
	userEmail string,
) (clientSecret, providerRef string, err error) {
	
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amount),
		Currency: stripe.String(strings.ToLower(string(currency))),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}
	params.AddMetadata("deposit_id", depositID)
	
	if userEmail != "" {
		params.ReceiptEmail = stripe.String(userEmail)
	}

	params.Context = ctx

	pi, err := paymentintent.New(params)
	if err != nil {
		return "", "", fmt.Errorf("failed to create stripe payment intent: %w", err)
	}

	return pi.ClientSecret, pi.ID, nil
}
