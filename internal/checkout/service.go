package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"drexa/internal/auth"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/checkout/session"
	"github.com/stripe/stripe-go/v78/customer"
	"github.com/stripe/stripe-go/v78/product"
	"github.com/stripe/stripe-go/v78/webhook"
)

type CheckoutService interface {
	CreateProduct(ctx context.Context) (string, error)
	CreateCheckoutSession(ctx context.Context, userID string) (string, error)
	HandleWebhook(ctx context.Context, payload []byte, signature string) error
}

type checkoutService struct {
	stripeSecretKey string
	webhookSecret   string
	appURL          string
	purchaseRepo    PurchaseRepository
	userRepo        auth.UserRepository
}

func NewCheckoutService(
	stripeSecretKey, webhookSecret, appURL string,
	purchaseRepo PurchaseRepository,
	userRepo auth.UserRepository,
) CheckoutService {
	stripe.Key = stripeSecretKey
	return &checkoutService{
		stripeSecretKey: stripeSecretKey,
		webhookSecret:   webhookSecret,
		appURL:          appURL,
		purchaseRepo:    purchaseRepo,
		userRepo:        userRepo,
	}
}

func (s *checkoutService) CreateProduct(ctx context.Context) (string, error) {
	// The blueprint requires tax_code txcd_10103100, unit amount 1000, and usd
	params := &stripe.ProductParams{
		Name:        stripe.String("Hamlet (e-book)"),
		Description: stripe.String("A Shakespearean tragedy"),
		TaxCode:     stripe.String("txcd_10103100"),
		DefaultPriceData: &stripe.ProductDefaultPriceDataParams{
			UnitAmount: stripe.Int64(1000),
			Currency:   stripe.String("usd"),
		},
	}
	// The blueprint explicitly says to use this version for the /v1/products call
	params.AddExtra("stripe-version", "2026-02-25.preview")
	params.Context = ctx

	p, err := product.New(params)
	if err != nil {
		return "", fmt.Errorf("checkout_svc: failed to create product: %w", err)
	}

	return p.ID, nil
}

func (s *checkoutService) getOrCreateCustomer(ctx context.Context, user *auth.User) (string, error) {
	if user.StripeCustomerID != "" {
		return user.StripeCustomerID, nil
	}

	params := &stripe.CustomerParams{
		Email: stripe.String(user.Email),
		Name:  stripe.String(user.Email), // or phone, depending on fields
	}
	params.Context = ctx

	c, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("checkout_svc: failed to create stripe customer: %w", err)
	}

	err = s.userRepo.UpdateStripeCustomerID(ctx, user.UserID, c.ID)
	if err != nil {
		return "", fmt.Errorf("checkout_svc: failed to update user stripe ID: %w", err)
	}

	return c.ID, nil
}

func (s *checkoutService) CreateCheckoutSession(ctx context.Context, userID string) (string, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return "", err
	}

	customerID, err := s.getOrCreateCustomer(ctx, user)
	if err != nil {
		return "", err
	}

	// For the demonstration, we'll assume the product has been created and we find its price.
	// Normally we'd store the Product ID or Price ID. We can fetch active products.
	// To simplify, let's just create an ad-hoc price data. Wait, the blueprint says
	// "use the created customer ID when creating a subscription" - actually the blueprint just says
	// "checkout/sessions", not subscriptions.
	// To strictly follow, we need the price ID of the product we created, or we can use PriceData.
	// We will use PriceData for simplicity and matching the blueprint.
	
	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		Mode:     stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String("usd"),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("Hamlet (e-book)"),
					},
					UnitAmount: stripe.Int64(1000),
				},
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(fmt.Sprintf("%s/checkout/success?session_id={CHECKOUT_SESSION_ID}", s.appURL)),
		CancelURL:  stripe.String(fmt.Sprintf("%s/checkout/cancel", s.appURL)),
	}
	
	// Add the managed_payments preview parameter as requested in the blueprint
	params.AddExtra("managed_payments[enabled]", "true")
	params.AddExtra("stripe-version", "2026-02-25.preview")
	params.Context = ctx

	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("checkout_svc: failed to create session: %w", err)
	}

	purchase := &Purchase{
		PurchaseID:      uuid.New().String(),
		UserID:          userID,
		StripeSessionID: sess.ID,
		Status:          StatusPending,
	}

	if err := s.purchaseRepo.Create(ctx, purchase); err != nil {
		return "", err
	}

	return sess.URL, nil
}

func (s *checkoutService) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	event, err := webhook.ConstructEvent(payload, signature, s.webhookSecret)
	if err != nil {
		return fmt.Errorf("checkout_svc: invalid webhook signature: %w", err)
	}

	if event.Type == "checkout.session.completed" {
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			return fmt.Errorf("checkout_svc: failed to parse session: %w", err)
		}

		purchase, err := s.purchaseRepo.FindBySessionID(ctx, sess.ID)
		if err != nil {
			// Might be a different checkout session, ignore if not found
			if errors.Is(err, ErrPurchaseNotFound) {
				return nil
			}
			return fmt.Errorf("checkout_svc: failed to find purchase: %w", err)
		}

		err = s.purchaseRepo.UpdateStatus(ctx, purchase.PurchaseID, StatusCompleted)
		if err != nil {
			return fmt.Errorf("checkout_svc: failed to update purchase status: %w", err)
		}
	}

	return nil
}
