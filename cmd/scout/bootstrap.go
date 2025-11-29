package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Bootstrap test data (users, tenants, memberships)",
	Long:  `Create test users, tenants, and memberships for development/testing.`,
	RunE:  runBootstrap,
}

func init() {
	bootstrapCmd.Flags().StringP("email", "e", "test@example.com", "User email")
	bootstrapCmd.Flags().StringP("name", "n", "Test User", "User name")
	bootstrapCmd.Flags().StringP("tenant-name", "t", "Test Tenant", "Tenant name")
	bootstrapCmd.Flags().StringP("role", "r", "admin", "User role")
	bootstrapCmd.Flags().StringP("config", "c", "", "Path to config file")
}

func runBootstrap(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Get flags
	email, _ := cmd.Flags().GetString("email")
	name, _ := cmd.Flags().GetString("name")
	tenantName, _ := cmd.Flags().GetString("tenant-name")
	role, _ := cmd.Flags().GetString("role")

	ctx := context.Background()

	// Initialize repositories
	tenantRepo := repository.NewTenantRepository(db.DB, logger)
	userRepo := repository.NewUserRepository(db.DB, logger)
	membershipRepo := repository.NewMembershipRepository(db.DB, logger)

	// Create tenant
	tenantID := uuid.New()
	tenant := &models.Tenant{
		TenantID:  tenantID,
		Name:      tenantName,
		CreatedAt: time.Now(),
	}
	if err := tenantRepo.Create(ctx, tenant); err != nil {
		logger.Info("Tenant may already exist, continuing...", zap.Error(err))
	} else {
		fmt.Fprintf(os.Stderr, "Created tenant: %s (%s)\n", tenantName, tenantID.String())
	}

	// Check if user already exists
	existingUser, err := tenantRepo.GetUserByEmail(ctx, email)
	var userID uuid.UUID
	if err == nil {
		userID = existingUser.UserID
		fmt.Fprintf(os.Stderr, "User already exists: %s (%s)\n", email, userID.String())
	} else {
		// Create user
		userID = uuid.New()
		user := &models.User{
			UserID:    userID,
			Email:     email,
			Name:      name,
			CreatedAt: time.Now(),
		}
		if err := userRepo.Create(ctx, user); err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Created user: %s (%s)\n", email, userID.String())
	}

	// Create membership
	membership := &models.Membership{
		MembershipID: uuid.New(),
		TenantID:     tenantID,
		UserID:       userID,
		Role:         role,
		CreatedAt:    time.Now(),
	}
	if err := membershipRepo.Create(ctx, membership); err != nil {
		logger.Info("Membership may already exist, continuing...", zap.Error(err))
	} else {
		fmt.Fprintf(os.Stderr, "Created membership: %s role for user %s in tenant %s\n", role, userID.String(), tenantID.String())
	}

	fmt.Fprintf(os.Stderr, "\nBootstrap complete!\n")
	fmt.Fprintf(os.Stderr, "You can now generate a token with:\n")
	fmt.Fprintf(os.Stderr, "  ./bin/scout token --user-id %s --tenant-id %s --role %s\n", userID.String(), tenantID.String(), role)

	return nil
}
