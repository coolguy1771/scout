package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
	"github.com/coolguy1771/scout/internal/worker"
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

	// Initialize worker
	w, err := worker.New(cfg, db, logger)
	if err != nil {
		logger.Fatal("Failed to initialize worker", zap.Error(err))
	}

	// Start worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		logger.Info("Starting worker service")
		if err := w.Start(ctx); err != nil {
			logger.Fatal("Worker failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down worker...")
	cancel()

	// Give worker time to finish current jobs
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := w.Shutdown(shutdownCtx); err != nil {
		logger.Error("Worker shutdown error", zap.Error(err))
	}

	logger.Info("Worker exited")
}
