package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/coolguy1771/scout/internal/config"
)

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
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Unauthorized: missing Authorization header", http.StatusUnauthorized)
				return
			}

			// Remove "Bearer " prefix
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == authHeader {
				http.Error(w, "Unauthorized: invalid Authorization header format", http.StatusUnauthorized)
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
				http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			if !token.Valid {
				http.Error(w, "Unauthorized: token is not valid", http.StatusUnauthorized)
				return
			}

			// Validate issuer if configured
			if cfg.Issuer != "" && claims.Issuer != cfg.Issuer {
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
