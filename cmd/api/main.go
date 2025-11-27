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

	// Initialize router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
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

	// Health check
	// @Summary Health check
	// @Description Check if the API is running
	// @Tags health
	// @Produce plain
	// @Success 200 {string} string "OK"
	// @Router /health [get]
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

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
