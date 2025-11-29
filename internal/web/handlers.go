package web

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/middleware/auth"
)

// WebHandler handles web UI requests
type WebHandler struct {
	logger *zap.Logger
	config *config.Config
}

// NewWebHandler creates a new web handler
func NewWebHandler(logger *zap.Logger, cfg *config.Config) *WebHandler {
	return &WebHandler{
		logger: logger,
		config: cfg,
	}
}

// LoginPage renders the login page
func (h *WebHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title": "Login - Scout",
	}
	renderTemplate(w, "login", data, h.logger)
}

// Login handles login form submission
func (h *WebHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// For MVP, we'll accept a token directly or generate one from user/tenant IDs
	token := r.FormValue("token")
	if token == "" {
		// Try to generate a token from user/tenant IDs
		userID := r.FormValue("user_id")
		tenantID := r.FormValue("tenant_id")
		role := r.FormValue("role")
		if role == "" {
			role = "analyst"
		}

		if userID == "" || tenantID == "" {
			renderTemplate(w, "login", map[string]interface{}{
				"Title": "Login - Scout",
				"Error": "User ID and Tenant ID are required",
			}, h.logger)
			return
		}

		// Generate token
		token = h.generateToken(userID, tenantID, role)
		if token == "" {
			renderTemplate(w, "login", map[string]interface{}{
				"Title": "Login - Scout",
				"Error": "Failed to generate token",
			}, h.logger)
			return
		}
	}

	// Set token in HTTP-only cookie
	// Use SameSiteLax to allow cookie on redirect after login
	cookie := &http.Cookie{
		Name:     "scout_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.API.Env == "production",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   h.config.JWT.Expires * 3600, // Convert hours to seconds
	}
	http.SetCookie(w, cookie)
	
	h.logger.Info("Login successful, cookie set", 
		zap.String("path", cookie.Path),
		zap.Bool("httpOnly", cookie.HttpOnly),
		zap.Int("maxAge", cookie.MaxAge))

	// Redirect to dashboard
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout handles logout
func (h *WebHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Clear token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "scout_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.API.Env == "production",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Dashboard renders the main dashboard
func (h *WebHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	// Get user info from context (set by auth middleware)
	// If middleware didn't set these, user is not authenticated
	userID := auth.GetUserID(r.Context())
	tenantID := auth.GetTenantID(r.Context())
	role := auth.GetRole(r.Context())

	h.logger.Info("Dashboard accessed",
		zap.String("userID", userID),
		zap.String("tenantID", tenantID),
		zap.String("role", role),
		zap.String("path", r.URL.Path))

	if userID == "" || tenantID == "" {
		// Not authenticated, redirect to login
		h.logger.Warn("Dashboard access denied - missing user/tenant ID")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	data := map[string]interface{}{
		"Title":    "Dashboard - Scout",
		"UserID":   userID,
		"TenantID": tenantID,
		"Role":     role,
	}

	renderTemplate(w, "dashboard", data, h.logger)
}

// generateToken generates a JWT token
func (h *WebHandler) generateToken(userID, tenantID, role string) string {
	claims := jwt.MapClaims{
		"userId":   userID,
		"tenantId": tenantID,
		"role":     role,
		"iss":      h.config.JWT.Issuer,
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Duration(h.config.JWT.Expires) * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.config.JWT.Secret))
	if err != nil {
		h.logger.Error("Failed to generate token", zap.Error(err))
		return ""
	}

	return tokenString
}

// RegisterWebRoutes registers web UI routes
func RegisterWebRoutes(r chi.Router, logger *zap.Logger, cfg *config.Config) {
	handler := NewWebHandler(logger, cfg)

	// Public routes
	r.Group(func(r chi.Router) {
		r.Get("/login", handler.LoginPage)
		r.Post("/login", handler.Login)
	})

	// Protected routes (require auth)
	// Cookie middleware is already applied at the top level in api.go
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(cfg.JWT))
		r.Get("/", handler.Dashboard)
		r.Post("/logout", handler.Logout)
	})
}

