package kyc

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
)

type Handler struct {
	usecase      Usecase
	adminUsecase AdminUsecase
	diditUsecase DiditUsecase // optional; nil when Didit is not configured
	getUserID    func(r *http.Request) string
}

func NewHandler(uc Usecase, admin AdminUsecase, didit DiditUsecase, getUserID func(r *http.Request) string) *Handler {
	return &Handler{usecase: uc, adminUsecase: admin, diditUsecase: didit, getUserID: getUserID}
}

// POST /api/v1/kyc/submit
func (h *Handler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		FullName  string `json:"full_name"`
		IDNumber  string `json:"id_number"`
		IDType    string `json:"id_type"`
		FileURL   string `json:"file_url"`
		SelfieURL string `json:"selfie_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.FullName == "" || body.IDNumber == "" || body.IDType == "" || body.FileURL == "" {
		http.Error(w, "full_name, id_number, id_type, file_url required", http.StatusBadRequest)
		return
	}

	sub := &Submission{
		FullName:  body.FullName,
		IDNumber:  body.IDNumber,
		IDType:    body.IDType,
		FileURL:   body.FileURL,
		SelfieURL: body.SelfieURL,
	}

	if err := h.usecase.Submit(r.Context(), userID, sub); err != nil {
		switch {
		case errors.Is(err, ErrAlreadySubmitted):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, ErrUserNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			log.Ctx(r.Context()).Error().Err(err).Msg("kyc: submit failed")
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "KYC submission received"})
}

// GET /api/v1/kyc/status
func (h *Handler) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sub, err := h.usecase.GetByUserID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "no kyc submission found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

// POST /api/v1/kyc/didit/session
// Creates a Didit verification session for the authenticated user and returns
// the hosted url for the frontend SDK / iframe / redirect.
func (h *Handler) HandleStartDiditVerification(w http.ResponseWriter, r *http.Request) {
	if h.diditUsecase == nil {
		http.Error(w, "identity verification not configured", http.StatusServiceUnavailable)
		return
	}

	userID := h.getUserID(r)
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	session, err := h.diditUsecase.StartVerification(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		log.Ctx(r.Context()).Error().Err(err).Msg("kyc: didit start verification failed")
		http.Error(w, "could not start verification", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	// Return only what the client needs (url + session_id), never the api key.
	json.NewEncoder(w).Encode(map[string]string{
		"url":        session.URL,
		"session_id": session.SessionID,
	})
}

// POST /api/v1/kyc/didit/webhook  (public — authenticated by HMAC signature)
// Verifies X-Signature-V2 + timestamp freshness, dedupes on event_id, then
// applies the decision. Always returns 2xx quickly to Didit.
func (h *Handler) HandleDiditWebhook(w http.ResponseWriter, r *http.Request) {
	if h.diditUsecase == nil {
		http.Error(w, "identity verification not configured", http.StatusServiceUnavailable)
		return
	}

	const maxBody = int64(1 << 20)
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusServiceUnavailable)
		return
	}

	sig := r.Header.Get("X-Signature-V2")
	ts, _ := strconv.ParseInt(r.Header.Get("X-Timestamp"), 10, 64)

	// Verify freshness + HMAC before trusting any field in the body.
	if err := h.diditUsecase.Service().VerifyWebhook(raw, sig, ts); err != nil {
		log.Ctx(r.Context()).Warn().Err(err).Msg("kyc: didit webhook verification failed")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var event DiditWebhookEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Idempotency — dedupe on event_id (unique per delivery attempt).
	if event.EventID != "" {
		processed, err := h.diditUsecase.Repo().IsEventProcessed(r.Context(), event.EventID)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("kyc: didit idempotency check failed")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if processed {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	if err := h.diditUsecase.HandleWebhook(r.Context(), &event); err != nil {
		// 5xx so Didit retries (~1 min, then ~4 min).
		log.Ctx(r.Context()).Error().Err(err).Msg("kyc: didit webhook processing failed")
		http.Error(w, "processing failed", http.StatusInternalServerError)
		return
	}

	if event.EventID != "" {
		if err := h.diditUsecase.Repo().MarkEventProcessed(r.Context(), event.EventID); err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("kyc: didit mark processed failed")
		}
	}

	w.WriteHeader(http.StatusOK)
}

// GET /api/v1/admin/kyc?status=pending
func (h *Handler) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	statusParam := r.URL.Query().Get("status")
	if statusParam == "" {
		statusParam = "pending"
	}
	status := Status(statusParam)

	subs, err := h.adminUsecase.ListByStatus(r.Context(), status)
	if err != nil {
		log.Ctx(r.Context()).Error().Err(err).Msg("admin_kyc: list failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subs)
}

// GET /api/v1/admin/kyc/{id}
func (h *Handler) HandleAdminGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sub, err := h.adminUsecase.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

// POST /api/v1/admin/kyc/{id}/approve
func (h *Handler) HandleAdminApprove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reviewedBy := h.getUserID(r)

	if err := h.adminUsecase.Approve(r.Context(), id, reviewedBy); err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		log.Ctx(r.Context()).Error().Err(err).Msg("admin_kyc: approve failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "approved"})
}

// POST /api/v1/admin/kyc/{id}/reject
func (h *Handler) HandleAdminReject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reviewedBy := h.getUserID(r)

	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Reason == "" {
		http.Error(w, "reason required", http.StatusBadRequest)
		return
	}

	if err := h.adminUsecase.Reject(r.Context(), id, reviewedBy, body.Reason); err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		log.Ctx(r.Context()).Error().Err(err).Msg("admin_kyc: reject failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "rejected"})
}
