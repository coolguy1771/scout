package auth

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
)

var cookieLogger *zap.Logger

// SetCookieLogger sets the logger for cookie middleware debugging
func SetCookieLogger(logger *zap.Logger) {
	cookieLogger = logger
}

// CookieMiddleware extracts JWT token from cookie and sets it in Authorization header
// This allows web requests to use cookie-based auth while API requests use Bearer tokens
func CookieMiddleware(cfg config.JWTConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if Authorization header is already set
			if r.Header.Get("Authorization") == "" {
				// Try to get token from cookie
				cookie, err := r.Cookie("scout_token")
				if err == nil && cookie != nil && cookie.Value != "" {
					// Set Authorization header from cookie
					r.Header.Set("Authorization", "Bearer "+cookie.Value)
					if cookieLogger != nil {
						cookieLogger.Debug("Extracted token from cookie",
							zap.String("path", r.URL.Path),
							zap.Int("tokenLength", len(cookie.Value)))
					}
				} else if cookieLogger != nil {
					cookieLogger.Debug("No cookie found",
						zap.String("path", r.URL.Path),
						zap.Error(err))
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
