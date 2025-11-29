package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/spf13/cobra"
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
	"github.com/coolguy1771/scout/internal/web"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Start the API server",
	Long:  `Start the Scout API server that handles REST API requests and serves vector tiles.`,
	RunE:  runAPI,
}

func init() {
	apiCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	apiCmd.Flags().StringP("config", "c", "", "Path to config file")
}

func runAPI(cmd *cobra.Command, args []string) error {
	// Get config file path from flag
	configPath, _ := cmd.Flags().GetString("config")

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override port if flag is set
	if port, err := cmd.Flags().GetInt("port"); err == nil && cmd.Flags().Changed("port") {
		cfg.API.Port = port
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

	// Cookie middleware for web UI (converts cookie to Authorization header)
	// This is safe to apply globally as it only adds headers if cookies exist
	auth.SetCookieLogger(logger)
	auth.SetAuthLogger(logger)
	r.Use(auth.CookieMiddleware(cfg.JWT))

	// Health check endpoints
	healthHandler := api.NewHealthHandler(db, searchClient, s3Storage, cfg, logger)
	r.Get("/health", healthHandler.Health)
	r.Get("/health/live", healthHandler.Live)
	r.Get("/health/ready", healthHandler.Ready)

	// Metrics endpoint
	r.Handle("/metrics", metrics.Handler())

	// Swagger UI
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Static files (CSS, JS, etc.)
	workDir, _ := os.Getwd()
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(workDir+"/web/static"))))

	// Web UI routes (must be before API routes to catch root path)
	web.RegisterWebRoutes(r, logger, cfg)

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
		return err
	}

	logger.Info("Server exited")
	return nil
}
