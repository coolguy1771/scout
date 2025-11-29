package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
)

var authLogger *zap.Logger

// SetAuthLogger sets the logger for auth middleware debugging
func SetAuthLogger(logger *zap.Logger) {
	authLogger = logger
}

type contextKey string

const (
	UserIDKey   contextKey = "userID"
	TenantIDKey contextKey = "tenantID"
	RoleKey     contextKey = "role"
)

// Claims represents JWT claims
type Claims struct {
	UserID   string `json:"userId"`
	TenantID string `json:"tenantId"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Middleware validates JWT tokens and extracts user context
func Middleware(cfg config.JWTConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenString string

			// Try to get token from Authorization header first
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				// Remove "Bearer " prefix
				tokenString = strings.TrimPrefix(authHeader, "Bearer ")
				if tokenString == authHeader {
					// Invalid format, but continue to check cookie
					tokenString = ""
				}
			}

			// If no token in header, try cookie
			if tokenString == "" {
				cookie, err := r.Cookie("scout_token")
				if err == nil && cookie.Value != "" {
					tokenString = cookie.Value
				}
			}

			// If still no token, return error
			if tokenString == "" {
				if authLogger != nil {
					_, hasCookie := r.Cookie("scout_token")
					authLogger.Debug("No token found",
						zap.String("path", r.URL.Path),
						zap.Bool("hasAuthHeader", authHeader != ""),
						zap.Bool("hasCookie", hasCookie == nil))
				}
				// For web routes, redirect to login
				if r.URL.Path == "/" || !strings.HasPrefix(r.URL.Path, "/api/") {
					http.Redirect(w, r, "/login", http.StatusSeeOther)
					return
				}
				http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
				return
			}

			// Parse and validate token
			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
				// Validate signing method
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, errors.New("unexpected signing method")
				}
				return []byte(cfg.Secret), nil
			})

			if err != nil {
				// Log the error for debugging
				if authLogger != nil {
					authLogger.Warn("JWT validation failed",
						zap.String("path", r.URL.Path),
						zap.Error(err))
				}
				if r.URL.Path == "/" {
					// For root path, redirect to login instead of showing error
					http.Redirect(w, r, "/login", http.StatusSeeOther)
					return
				}
				http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			if !token.Valid {
				if authLogger != nil {
					authLogger.Warn("JWT token is not valid", zap.String("path", r.URL.Path))
				}
				if r.URL.Path == "/" {
					http.Redirect(w, r, "/login", http.StatusSeeOther)
					return
				}
				http.Error(w, "Unauthorized: token is not valid", http.StatusUnauthorized)
				return
			}

			// Validate issuer if configured
			if cfg.Issuer != "" && claims.Issuer != cfg.Issuer {
				if authLogger != nil {
					authLogger.Warn("JWT issuer mismatch",
						zap.String("path", r.URL.Path),
						zap.String("expected", cfg.Issuer),
						zap.String("got", claims.Issuer))
				}
				if r.URL.Path == "/" {
					http.Redirect(w, r, "/login", http.StatusSeeOther)
					return
				}
				http.Error(w, "Unauthorized: invalid token issuer", http.StatusUnauthorized)
				return
			}

			// Check expiration
			if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
				http.Error(w, "Unauthorized: token expired", http.StatusUnauthorized)
				return
			}

			// Set context values
			ctx := r.Context()
			if claims.UserID != "" {
				ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
			}
			if claims.TenantID != "" {
				ctx = context.WithValue(ctx, TenantIDKey, claims.TenantID)
			}
			if claims.Role != "" {
				ctx = context.WithValue(ctx, RoleKey, claims.Role)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID extracts user ID from context
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// GetTenantID extracts tenant ID from context
func GetTenantID(ctx context.Context) string {
	if tenantID, ok := ctx.Value(TenantIDKey).(string); ok {
		return tenantID
	}
	return ""
}

// GetRole extracts role from context
func GetRole(ctx context.Context) string {
	if role, ok := ctx.Value(RoleKey).(string); ok {
		return role
	}
	return ""
}

// RequireRole middleware ensures user has required role
func RequireRole(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := GetRole(r.Context())
			if role == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Simple role check (viewer < analyst < admin)
			roleHierarchy := map[string]int{
				"viewer":  1,
				"analyst": 2,
				"admin":   3,
			}

			userLevel, ok := roleHierarchy[role]
			requiredLevel, requiredOk := roleHierarchy[requiredRole]

			if !ok || !requiredOk || userLevel < requiredLevel {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
