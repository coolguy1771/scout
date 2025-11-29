// Package main Scout API
// @title Scout API
// @version 1.0
// @description API for Scout land parcel analysis platform
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email support@scout.com
// @license.name AGPL-3.0
// @license.url https://www.gnu.org/licenses/agpl-3.0.html
// @host localhost:8080
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter the token with the `Bearer: ` prefix, e.g. "Bearer abc123..."
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	httpSwagger "github.com/swaggo/http-swagger"
	"go.uber.org/zap"

	_ "github.com/coolguy1771/scout/docs" // swagger docs
	"github.com/coolguy1771/scout/internal/api"
	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
	"github.com/coolguy1771/scout/internal/middleware/auth"
	"github.com/coolguy1771/scout/internal/middleware/logging"
	"github.com/coolguy1771/scout/internal/middleware/metrics"
	"github.com/coolguy1771/scout/internal/middleware/ratelimit"
	"github.com/coolguy1771/scout/internal/search/opensearch"
	"github.com/coolguy1771/scout/internal/storage"
)

func main() {
	// Parse flags
	configPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	var logger *zap.Logger
	if cfg.LogFormat == "json" {
		logger, _ = zap.NewProduction()
	} else {
		logger, _ = zap.NewDevelopment()
	}
	defer logger.Sync()

	// Initialize database
	db, err := database.New(cfg.Database.DSN())
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Initialize OpenSearch client (optional)
	var searchClient *opensearch.Client
	if cfg.Search.Enabled {
		searchClient, err = opensearch.NewClient(&cfg.Search, logger)
		if err != nil {
			logger.Warn("Failed to initialize OpenSearch client", zap.Error(err))
		}
	}

	// Initialize S3 storage (optional)
	var s3Storage *storage.S3Storage
	if cfg.S3.AccessKeyID != "" && cfg.S3.SecretAccessKey != "" {
		s3Storage, err = storage.NewS3Storage(cfg.S3, logger)
		if err != nil {
			logger.Warn("Failed to initialize S3 storage", zap.Error(err))
		}
	}

	// Initialize router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(metrics.Middleware) // Metrics must be before logging to capture all requests
	r.Use(logging.Middleware(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS
	corsOptions := cors.Options{
		AllowedOrigins:   cfg.API.CORS.AllowedOrigins,
		AllowedMethods:   cfg.API.CORS.AllowedMethods,
		AllowedHeaders:   cfg.API.CORS.AllowedHeaders,
		ExposedHeaders:   cfg.API.CORS.ExposedHeaders,
		AllowCredentials: cfg.API.CORS.AllowCredentials,
		MaxAge:           cfg.API.CORS.MaxAge,
	}
	r.Use(cors.Handler(corsOptions))

	// Rate limiting (if enabled)
	if cfg.API.RateLimit.Enabled {
		limiter := ratelimit.NewRateLimiter(
			cfg.API.RateLimit.GlobalLimit,
			cfg.API.RateLimit.TenantLimit,
			cfg.API.RateLimit.Burst,
		)
		r.Use(limiter.Middleware)
	}

	// Health check endpoints
	healthHandler := api.NewHealthHandler(db, searchClient, s3Storage, cfg, logger)
	r.Get("/health", healthHandler.Health)
	r.Get("/health/live", healthHandler.Live)
	r.Get("/health/ready", healthHandler.Ready)

	// Metrics endpoint
	r.Get("/metrics", metrics.Handler())

	// Swagger UI
	r.Get("/swagger", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/index.html", http.StatusMovedPermanently)
	})
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Public routes
		r.Group(func(r chi.Router) {
			// Add public routes here
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(cfg.JWT))
			api.RegisterRoutes(r, db, logger, cfg)
		})
	})

	// Tile routes (may need auth for private layers)
	r.Route("/tiles", func(r chi.Router) {
		api.RegisterTileRoutes(r, db, logger, cfg)
	})

	// Start server
	addr := fmt.Sprintf(":%d", cfg.API.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		logger.Info("Starting API server", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}
