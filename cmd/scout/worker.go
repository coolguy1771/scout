package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
	"github.com/coolguy1771/scout/internal/worker"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Start the worker service",
	Long:  `Start the Scout worker service that processes background jobs (exports, tiling, feature computation).`,
	RunE:  runWorker,
}

func init() {
	workerCmd.Flags().StringP("config", "c", "", "Path to config file")
}

func runWorker(cmd *cobra.Command, args []string) error {
	// Get config file path from flag
	configPath, _ := cmd.Flags().GetString("config")

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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
		return err
	}

	logger.Info("Worker exited")
	return nil
}
