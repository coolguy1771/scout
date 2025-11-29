package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/coolguy1771/scout/internal/config"
)

func TestMiddleware_ValidToken(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:  "test-secret-key-for-testing-purposes-only",
		Issuer:  "scout",
		Expires: 24,
	}

	// Create a valid token
	claims := jwt.MapClaims{
		"userId":   "123e4567-e89b-12d3-a456-426614174000",
		"tenantId": "123e4567-e89b-12d3-a456-426614174001",
		"role":     "analyst",
		"iss":      "scout",
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.Secret))
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	// Create middleware
	middleware := Middleware(cfg)

	// Create handler that checks context
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		tenantID := GetTenantID(r.Context())
		role := GetRole(r.Context())

		if userID == "" || tenantID == "" || role == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))

	// Create request with token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestMiddleware_MissingToken(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:  "test-secret",
		Issuer:  "scout",
		Expires: 24,
	}

	middleware := Middleware(cfg)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}



