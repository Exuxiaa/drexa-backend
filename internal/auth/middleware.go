package auth

import (
	"context"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
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

<<<<<<< HEAD
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
=======
// RateLimitMiddleware throttles requests per client IP using a Redis-backed
// fixed window. `name` namespaces the counter so different routes (login,
// register) are limited independently. On limit breach it returns 429.
//
// It fails open: if Redis is unavailable the request is allowed through (and
// logged) so a cache outage cannot lock every user out of authentication.
func RateLimitMiddleware(limiter RateLimiter, name string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := name + ":" + clientIP(r)

			allowed, err := limiter.Allow(r.Context(), key, limit, window)
			if err != nil {
				log.Printf("ratelimit: %s: %v (allowing request)", name, err)
				next.ServeHTTP(w, r)
				return
			}
			if !allowed {
				sendJSON(w, http.StatusTooManyRequests, MessageResponse{Error: "too many attempts, please try again later"})
				return
			}

>>>>>>> e448e44364a4225c0819ff59d6af60c71d778498
			next.ServeHTTP(w, r)
		})
	}
}

<<<<<<< HEAD
=======
// clientIP extracts the originating IP. It honours the left-most X-Forwarded-For
// entry set by a trusted reverse proxy (nginx ingress) and falls back to the
// raw connection address otherwise.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

>>>>>>> e448e44364a4225c0819ff59d6af60c71d778498
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
