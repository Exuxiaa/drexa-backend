package middleware

import (
	"net/http"
	"strings"
)

// CORS returns middleware that handles browser cross-origin requests from the
// frontend. Because the frontend calls the API with `credentials: 'include'`
// (cookie-based auth), the response MUST echo the specific request Origin and
// set Allow-Credentials: true — a wildcard "*" is rejected by browsers when
// credentials are sent.
//
// allowedOrigins is the exact list of origins permitted (e.g.
// "http://localhost:3000"). An entry of "*" allows any origin (reflected back),
// which is convenient for local development only.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAny := false
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAny = true
		}
		if o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				_, ok := allowed[origin]
				if ok || allowAny {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, Idempotency-Key, ngrok-skip-browser-warning")
					w.Header().Set("Access-Control-Max-Age", "600")
				}
			}

			// Short-circuit the CORS preflight before it reaches any route/auth.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
