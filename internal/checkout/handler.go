package checkout

import (
	"encoding/json"
	"io"
	"net/http"
)

type Handler struct {
	checkoutSvc CheckoutService
	getUserID   func(r *http.Request) string
}

func NewHandler(checkoutSvc CheckoutService, getUserID func(r *http.Request) string) *Handler {
	return &Handler{
		checkoutSvc: checkoutSvc,
		getUserID:   getUserID,
	}
}

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	// Require Admin role in reality, skipping for now
	productID, err := h.checkoutSvc.CreateProduct(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"product_id": productID})
}

func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	url, err := h.checkoutSvc.CreateCheckoutSession(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusServiceUnavailable)
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")
	
	err = h.checkoutSvc.HandleWebhook(r.Context(), payload, sigHeader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}
