package auth

import (
	"context"
	"net/http"
	"strings"
)

type ContextKey string

const UserClaimsKey ContextKey = "user_claims"

// JWTMiddleware validates the access token on every request.
// Token is read from the Authorization header first, then the access_token cookie.
func JWTMiddleware(tokenSvc TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
				return
			}

			claims, err := tokenSvc.ValidateAccessToken(r.Context(), token)
			if err != nil {
				sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
				return
			}
			// Reject restricted tokens (e.g. 2fa_challenge) from normal endpoints.
			if claims.Scope != "" {
				sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
				return
			}

			ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns a middleware that blocks users whose role is not in allowed.
func RequireRole(allowed ...UserRole) func(http.Handler) http.Handler {
	set := make(map[UserRole]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := UserFromContext(r.Context())
			if !ok {
				sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
				return
			}
			if _, ok := set[claims.Role]; !ok {
				sendJSON(w, http.StatusForbidden, MessageResponse{Error: "forbidden"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}


// UserFromContext retrieves the JWT claims injected by JWTMiddleware.
func UserFromContext(ctx context.Context) (*JWTClaims, bool) {
	claims, ok := ctx.Value(UserClaimsKey).(*JWTClaims)
	return claims, ok
}

// RequireKycLevel returns a middleware that blocks users whose KycLevel is below minLevel.
func RequireKycLevel(minLevel int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := UserFromContext(r.Context())
			if !ok {
				sendJSON(w, http.StatusUnauthorized, MessageResponse{Error: "unauthorized"})
				return
			}
			if claims.KycLevel < minLevel {
				sendJSON(w, http.StatusForbidden, MessageResponse{Error: "kyc level insufficient"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractBearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie("access_token"); err == nil {
		return c.Value
	}
	return ""
}
