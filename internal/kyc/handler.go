package kyc

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rs/zerolog/log"
)

type Handler struct {
	usecase      Usecase
	adminUsecase AdminUsecase
	getUserID    func(r *http.Request) string
}

func NewHandler(uc Usecase, admin AdminUsecase, getUserID func(r *http.Request) string) *Handler {
	return &Handler{usecase: uc, adminUsecase: admin, getUserID: getUserID}
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
