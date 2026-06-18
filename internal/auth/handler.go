package auth

import (
	"encoding/json"
<<<<<<< HEAD
=======
	"errors"
	"log"
>>>>>>> e448e44364a4225c0819ff59d6af60c71d778498
	"net/http"
)

// ─── DTOs ────────────────────────────────────────────────────────────────────

type RegisterRequest struct {
	Email    string `json:"email"`
<<<<<<< HEAD
	Phone    string `json:"phone"`
=======
	Password string `json:"password"`
	Username string `json:"username"`
}

type LoginRequest struct {
	Email    string `json:"email"`
>>>>>>> e448e44364a4225c0819ff59d6af60c71d778498
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type OTPRequest struct {
	OTP string `json:"otp"`
}

type PINRequest struct {
	PIN string `json:"pin"`
}

type MessageResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func sendJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func setAuthCookies(w http.ResponseWriter, access, refresh string) {
	http.SetCookie(w, &http.Cookie{
		Name: "access_token", Value: access, Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 900,
	})
	http.SetCookie(w, &http.Cookie{
		Name: "refresh_token", Value: refresh, Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 7 * 24 * 3600,
	})
}

func clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "access_token", Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", Path: "/", MaxAge: -1})
}

// ─── Public Auth Handlers ─────────────────────────────────────────────────────

<<<<<<< HEAD
func HandleRegister(u AuthUsecase) http.HandlerFunc {
=======
// HandleRegister creates a new account with email + password and issues JWT cookies.
func HandleRegister(u AuthUsecase, secureCookies bool) http.HandlerFunc {
>>>>>>> e448e44364a4225c0819ff59d6af60c71d778498
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

<<<<<<< HEAD
		user, err := u.Register(r.Context(), req.Email, req.Phone, req.Password)
		if err != nil {
			switch err {
			case ErrEmailAlreadyExists:
				sendJSON(w, http.StatusConflict, MessageResponse{Error: err.Error()})
			case ErrPhoneAlreadyExists:
				sendJSON(w, http.StatusConflict, MessageResponse{Error: err.Error()})
			default:
				sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "registration failed"})
			}
			return
		}

		sendJSON(w, http.StatusCreated, map[string]string{"user_id": user.UserID, "message": "registered successfully"})
	}
}

func HandleLogin(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		token, err := u.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "invalid credentials"})
			return
		}

		if token.RequiresTwoFA {
			sendJSON(w, http.StatusOK, map[string]any{
				"requires_2fa":    true,
				"challenge_token": token.ChallengeToken,
			})
			return
		}

		setAuthCookies(w, token.AccessToken, token.RefreshToken)
=======
		token, err := u.Register(r.Context(), req.Email, req.Password, req.Username)
		if err != nil {
			switch {
			case errors.Is(err, ErrEmailAlreadyExists):
				sendJSON(w, http.StatusConflict, MessageResponse{Error: "email already registered"})
			default:
				sendJSON(w, http.StatusBadRequest, MessageResponse{Error: err.Error()})
			}
			return
		}

		setAuthCookies(w, token.AccessToken, token.RefreshToken, secureCookies)
		sendJSON(w, http.StatusCreated, MessageResponse{Message: "registration successful"})
	}
}

// HandleLogin verifies email + password and issues JWT cookies.
func HandleLogin(u AuthUsecase, secureCookies bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		token, err := u.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			if errors.Is(err, ErrInvalidCredentials) {
				sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "invalid email or password"})
				return
			}
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "login failed"})
			return
		}

		setAuthCookies(w, token.AccessToken, token.RefreshToken, secureCookies)
>>>>>>> e448e44364a4225c0819ff59d6af60c71d778498
		sendJSON(w, http.StatusOK, MessageResponse{Message: "login successful"})
	}
}

func HandleLogout(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("refresh_token"); err == nil {
			_ = u.Logout(r.Context(), c.Value)
		}
		clearAuthCookies(w)
		sendJSON(w, http.StatusOK, MessageResponse{Message: "logged out"})
	}
}

func HandleRefreshToken(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("refresh_token")
		if err != nil {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "missing refresh token"})
			return
		}

		token, err := u.RefreshToken(r.Context(), c.Value)
		if err != nil {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "invalid or expired token"})
			return
		}

		setAuthCookies(w, token.AccessToken, token.RefreshToken)
		sendJSON(w, http.StatusOK, MessageResponse{Message: "token refreshed"})
	}
}

// ─── Protected Handlers (require JWTMiddleware) ───────────────────────────────

func HandleLogoutAll(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		if err := u.LogoutAll(r.Context(), claims.UserID); err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "failed to revoke sessions"})
			return
		}

		clearAuthCookies(w)
		sendJSON(w, http.StatusOK, MessageResponse{Message: "all sessions revoked"})
	}
}

func HandleChangePassword(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var req ChangePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		if err := u.ChangePassword(r.Context(), claims.UserID, req.OldPassword, req.NewPassword); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "password changed"})
	}
}

func HandleSendPhoneOTP(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		if err := u.SendPhoneOTP(r.Context(), claims.UserID); err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "failed to send OTP"})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "OTP sent"})
	}
}

func HandleVerifyPhoneOTP(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var req OTPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		if err := u.VerifyPhoneOTP(r.Context(), claims.UserID, req.OTP); err != nil {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "invalid or expired OTP"})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "phone verified"})
	}
}

func HandleSetTradingPIN(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var req PINRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		if err := u.SetTradingPIN(r.Context(), claims.UserID, req.PIN); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "trading PIN set"})
	}
}

func HandleVerifyTradingPIN(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var req PINRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}

		ok2, err := u.VerifyTradingPIN(r.Context(), claims.UserID, req.PIN)
		if err != nil || !ok2 {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "invalid PIN"})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "PIN verified"})
	}
}

// ─── 2FA Handlers ────────────────────────────────────────────────────────────

// HandleTwoFASetup initiates TOTP setup: returns the secret + QR URL.
// Client must call /auth/2fa/enable with a valid code to activate.
func HandleTwoFASetup(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		setup, err := u.InitiateTwoFA(r.Context(), claims.UserID)
		if err != nil {
			sendJSON(w, http.StatusInternalServerError, MessageResponse{Error: "failed to initiate 2FA"})
			return
		}

		sendJSON(w, http.StatusOK, map[string]string{
			"secret":      setup.Secret,
			"qr_code_url": setup.QRCodeURL,
		})
	}
}

// HandleTwoFAEnable confirms TOTP setup and activates 2FA for the account.
func HandleTwoFAEnable(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var body struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "code required"})
			return
		}

		if err := u.ConfirmTwoFA(r.Context(), claims.UserID, body.Code); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "2FA enabled"})
	}
}

// HandleTwoFADisable disables TOTP — requires a valid code as confirmation.
func HandleTwoFADisable(u AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
			return
		}

		var body struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "code required"})
			return
		}

		if err := u.DisableTwoFA(r.Context(), claims.UserID, body.Code); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: err.Error()})
			return
		}

		sendJSON(w, http.StatusOK, MessageResponse{Message: "2FA disabled"})
	}
}

// HandleLoginTwoFA completes the login flow for accounts with 2FA enabled.
// Accepts a challenge_token (from the initial login) + a TOTP code.
func HandleLoginTwoFA(u AuthUsecase, tokenSvc TokenService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChallengeToken string `json:"challenge_token"`
			Code           string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "invalid input"})
			return
		}
		if body.ChallengeToken == "" || body.Code == "" {
			sendJSON(w, http.StatusBadRequest, MessageResponse{Error: "challenge_token and code required"})
			return
		}

		claims, err := tokenSvc.ValidateAccessToken(r.Context(), body.ChallengeToken)
		if err != nil || claims.Scope != "2fa_challenge" {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "invalid or expired challenge token"})
			return
		}

		token, err := u.VerifyTwoFA(r.Context(), claims.UserID, body.Code)
		if err != nil {
			sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: err.Error()})
			return
		}

		setAuthCookies(w, token.AccessToken, token.RefreshToken)
		sendJSON(w, http.StatusOK, MessageResponse{Message: "login successful"})
	}
}
