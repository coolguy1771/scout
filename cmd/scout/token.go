package main

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/coolguy1771/scout/internal/config"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Generate a JWT token for testing",
	Long:  `Generate a JWT token with user, tenant, and role claims for API testing.`,
	RunE:  runToken,
}

func init() {
	tokenCmd.Flags().StringP("user-id", "u", "", "User ID (UUID)")
	tokenCmd.Flags().StringP("tenant-id", "t", "", "Tenant ID (UUID)")
	tokenCmd.Flags().StringP("role", "r", "analyst", "User role (viewer, analyst, admin)")
	tokenCmd.Flags().IntP("expires", "e", 24, "Expiration in hours")
	tokenCmd.Flags().StringP("config", "c", "", "Path to config file")
}

func runToken(cmd *cobra.Command, args []string) error {
	// Get config file path from flag
	configPath, _ := cmd.Flags().GetString("config")

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get flags
	userIDStr, _ := cmd.Flags().GetString("user-id")
	tenantIDStr, _ := cmd.Flags().GetString("tenant-id")
	role, _ := cmd.Flags().GetString("role")
	expiresHours, _ := cmd.Flags().GetInt("expires")

	// Generate UUIDs if not provided
	var userID, tenantID string
	if userIDStr == "" {
		userID = uuid.New().String()
	} else {
		userID = userIDStr
		// Validate UUID format
		if _, err := uuid.Parse(userID); err != nil {
			return fmt.Errorf("invalid user-id format: %w", err)
		}
	}

	if tenantIDStr == "" {
		tenantID = uuid.New().String()
	} else {
		tenantID = tenantIDStr
		// Validate UUID format
		if _, err := uuid.Parse(tenantID); err != nil {
			return fmt.Errorf("invalid tenant-id format: %w", err)
		}
	}

	// Validate role
	if role != "viewer" && role != "analyst" && role != "admin" {
		return fmt.Errorf("invalid role: must be viewer, analyst, or admin")
	}

	// Create claims
	claims := jwt.MapClaims{
		"userId":   userID,
		"tenantId": tenantID,
		"role":     role,
		"iss":      cfg.JWT.Issuer,
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Duration(expiresHours) * time.Hour).Unix(),
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token
	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	if err != nil {
		return fmt.Errorf("failed to sign token: %w", err)
	}

	// Print token
	fmt.Println(tokenString)
	fmt.Fprintf(os.Stderr, "\nToken generated:\n")
	fmt.Fprintf(os.Stderr, "  User ID:   %s\n", userID)
	fmt.Fprintf(os.Stderr, "  Tenant ID: %s\n", tenantID)
	fmt.Fprintf(os.Stderr, "  Role:      %s\n", role)
	fmt.Fprintf(os.Stderr, "  Expires:   %d hours\n", expiresHours)
	fmt.Fprintf(os.Stderr, "\nUse this token in the Authorization header:\n")
	fmt.Fprintf(os.Stderr, "  Authorization: Bearer %s\n", tokenString)

	return nil
}
