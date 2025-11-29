package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed database with sample data",
	Long:  `Seed the database with sample parcels, projects, saved searches, and other test data.`,
	RunE:  runSeed,
}

func init() {
	seedCmd.Flags().IntP("parcels", "p", 50, "Number of parcels to create")
	seedCmd.Flags().IntP("projects", "j", 3, "Number of projects to create")
	seedCmd.Flags().StringP("tenant-id", "t", "", "Tenant ID to use (creates new tenant if not provided)")
	seedCmd.Flags().StringP("user-id", "u", "", "User ID to use (creates new user if not provided)")
	seedCmd.Flags().StringP("config", "c", "", "Path to config file")
}

func runSeed(cmd *cobra.Command, args []string) error {
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
	numParcels, _ := cmd.Flags().GetInt("parcels")
	numProjects, _ := cmd.Flags().GetInt("projects")
	tenantIDStr, _ := cmd.Flags().GetString("tenant-id")
	userIDStr, _ := cmd.Flags().GetString("user-id")

	ctx := context.Background()

	// Initialize repositories
	tenantRepo := repository.NewTenantRepository(db.DB, logger)
	userRepo := repository.NewUserRepository(db.DB, logger)
	projectRepo := repository.NewProjectRepository(db.DB, logger)
	scoringProfileRepo := repository.NewScoringProfileRepository(db.DB, logger)

	// Get or create tenant
	var tenantID uuid.UUID
	if tenantIDStr != "" {
		tenantID, err = uuid.Parse(tenantIDStr)
		if err != nil {
			return fmt.Errorf("invalid tenant-id: %w", err)
		}
		// Verify tenant exists by querying directly
		var count int
		err = db.DB.GetContext(ctx, &count, "SELECT COUNT(*) FROM tenants WHERE tenant_id = $1", tenantID)
		if err != nil || count == 0 {
			return fmt.Errorf("tenant not found: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Using existing tenant: %s\n", tenantID.String())
	} else {
		// Create a new tenant
		tenantID = uuid.New()
		tenant := &models.Tenant{
			TenantID:  tenantID,
			Name:      "Seed Data Tenant",
			CreatedAt: time.Now(),
		}
		if err := tenantRepo.Create(ctx, tenant); err != nil {
			return fmt.Errorf("failed to create tenant: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Created tenant: %s\n", tenantID.String())
	}

	// Get or create user
	var userID uuid.UUID
	if userIDStr != "" {
		userID, err = uuid.Parse(userIDStr)
		if err != nil {
			return fmt.Errorf("invalid user-id: %w", err)
		}
		// Verify user exists by querying directly
		var count int
		err = db.DB.GetContext(ctx, &count, "SELECT COUNT(*) FROM users WHERE user_id = $1", userID)
		if err != nil || count == 0 {
			return fmt.Errorf("user not found: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Using existing user: %s\n", userID.String())
	} else {
		// Create a new user
		userID = uuid.New()
		user := &models.User{
			UserID:    userID,
			Email:     fmt.Sprintf("seed-%s@example.com", uuid.New().String()[:8]),
			Name:      "Seed Data User",
			CreatedAt: time.Now(),
		}
		if err := userRepo.Create(ctx, user); err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Created user: %s\n", userID.String())
	}

	// Create projects
	projectIDs := make([]uuid.UUID, 0, numProjects)
	for i := 0; i < numProjects; i++ {
		projectID := uuid.New()
		project := &models.Project{
			ProjectID: projectID,
			TenantID:  tenantID,
			Name:      fmt.Sprintf("Sample Project %d", i+1),
			CreatedBy: userID,
			CreatedAt: time.Now(),
		}
		if err := projectRepo.Create(ctx, project); err != nil {
			logger.Warn("Failed to create project", zap.Error(err), zap.Int("index", i))
			continue
		}
		projectIDs = append(projectIDs, projectID)
		fmt.Fprintf(os.Stderr, "Created project: %s\n", project.Name)
	}

	// Create scoring profiles
	scoringProfileID := uuid.New()
	weightsJSON, _ := json.Marshal(map[string]float64{
		"acres":           0.3,
		"dist_to_highway": 0.2,
		"dist_to_rail":    0.2,
		"zoning":          0.3,
	})
	scoringProfile := &models.ScoringProfile{
		ScoringProfileID:    scoringProfileID,
		TenantID:            tenantID,
		Name:                "Default Scoring Profile",
		Version:             1,
		WeightsJSON:         string(weightsJSON),
		ThresholdsJSON:      "{}",
		HardConstraintsJSON: "{}",
		CreatedBy:           userID,
		CreatedAt:           time.Now(),
	}
	if err := scoringProfileRepo.Create(ctx, scoringProfile); err != nil {
		logger.Warn("Failed to create scoring profile", zap.Error(err))
	} else {
		fmt.Fprintf(os.Stderr, "Created scoring profile: %s\n", scoringProfile.Name)
	}

	// Create saved searches
	if len(projectIDs) > 0 {
		queryJSON, _ := json.Marshal(map[string]interface{}{
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"must": []map[string]interface{}{
						{
							"range": map[string]interface{}{
								"acres": map[string]interface{}{
									"gte": 10,
									"lte": 100,
								},
							},
						},
					},
				},
			},
			"size": 20,
		})
		savedSearch := &models.SavedSearch{
			SavedSearchID:    uuid.New(),
			TenantID:         tenantID,
			ProjectID:        &projectIDs[0],
			Name:             "Sample Search: 10-100 acres",
			QueryJSON:        string(queryJSON),
			ScoringProfileID: &scoringProfileID,
			CreatedBy:        userID,
			CreatedAt:        time.Now(),
		}
		if err := projectRepo.CreateSavedSearch(ctx, savedSearch); err != nil {
			logger.Warn("Failed to create saved search", zap.Error(err))
		} else {
			fmt.Fprintf(os.Stderr, "Created saved search: %s\n", savedSearch.Name)
		}
	}

	// Create parcels with PostGIS geometries
	fmt.Fprintf(os.Stderr, "\nCreating %d parcels...\n", numParcels)
	parcelIDs := make([]uuid.UUID, 0, numParcels)

	// Sample locations (roughly around San Francisco Bay Area)
	baseLat := 37.7749
	baseLon := -122.4194

	rand.Seed(time.Now().UnixNano())
	for i := 0; i < numParcels; i++ {
		parcelID := uuid.New()

		// Generate a random polygon around a random point
		lat := baseLat + (rand.Float64()-0.5)*0.5  // ±0.25 degrees (~28km)
		lon := baseLon + (rand.Float64()-0.5)*0.5 // ±0.25 degrees

		// Create a simple square polygon (0.01 degrees ≈ 1.1km)
		size := 0.005 + rand.Float64()*0.01 // 0.5-1.5km
		polygonWKT := fmt.Sprintf(
			"POLYGON((%f %f, %f %f, %f %f, %f %f, %f %f))",
			lon-size, lat-size, // bottom-left
			lon+size, lat-size, // bottom-right
			lon+size, lat+size, // top-right
			lon-size, lat+size, // top-left
			lon-size, lat-size, // close polygon
		)

		// Calculate centroid
		centroidWKT := fmt.Sprintf("POINT(%f %f)", lon, lat)

		// Calculate acres (rough approximation: 1 degree ≈ 111km, so 0.01° ≈ 1.1km)
		acres := size * size * 111 * 111 * 247.105 // Convert km² to acres

		// Random zoning tags
		zoningOptions := [][]string{
			{"industrial", "heavy"},
			{"commercial", "mixed-use"},
			{"residential", "multi-family"},
			{"agricultural"},
			{"mixed-use", "commercial"},
		}
		zoningTags := zoningOptions[rand.Intn(len(zoningOptions))]

		// Insert parcel using raw SQL (needed for PostGIS)
		query := `
			INSERT INTO parcels (
				parcel_id, tenant_id, apn, acres, zoning_raw, zoning_tags,
				jurisdiction, state_fips, geom, centroid, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6::text[], $7, $8,
				ST_GeomFromText($9, 4326),
				ST_GeomFromText($10, 4326),
				NOW(), NOW()
			)
		`

		apn := fmt.Sprintf("APN-%06d", i+1)
		jurisdiction := []string{"San Francisco", "Oakland", "San Jose", "Berkeley", "Fremont"}[rand.Intn(5)]
		stateFIPS := "06" // California

		_, err := db.DB.ExecContext(ctx, query,
			parcelID,
			tenantID,
			apn,
			acres,
			fmt.Sprintf("Z-%s", zoningTags[0]),
			pq.Array(zoningTags), // PostgreSQL array
			jurisdiction,
			stateFIPS,
			polygonWKT,
			centroidWKT,
		)

		if err != nil {
			logger.Warn("Failed to create parcel", zap.Error(err), zap.Int("index", i))
			continue
		}

		parcelIDs = append(parcelIDs, parcelID)
		if (i+1)%10 == 0 {
			fmt.Fprintf(os.Stderr, "  Created %d/%d parcels...\r", i+1, numParcels)
		}
	}
	fmt.Fprintf(os.Stderr, "\nCreated %d parcels\n", len(parcelIDs))

	// Create some parcel features for a subset of parcels
	if len(parcelIDs) > 0 {
		fmt.Fprintf(os.Stderr, "\nCreating parcel features...\n")
		parcelFeatureRepo := repository.NewParcelFeatureRepository(db.DB, logger)
		featuresVersionID := uuid.New()

		// Create features for first 10 parcels
		numFeatures := 10
		if len(parcelIDs) < numFeatures {
			numFeatures = len(parcelIDs)
		}

		features := make([]models.ParcelFeature, 0, numFeatures)
		for i := 0; i < numFeatures; i++ {
			features = append(features, models.ParcelFeature{
				ParcelID:          parcelIDs[i],
				FeaturesVersionID: featuresVersionID,
				InFloodplain:      rand.Float64() < 0.2, // 20% chance
				InWetlands:        rand.Float64() < 0.1, // 10% chance
				InProtectedLand:   rand.Float64() < 0.15, // 15% chance
				DistToHighwayM:    rand.Float64() * 5000,  // 0-5km
				DistToRailM:       rand.Float64() * 10000, // 0-10km
				DistToAirportM:    rand.Float64() * 20000, // 0-20km
				DistToPowerLineM:  rand.Float64() * 3000,  // 0-3km
				DistToSubstationM: rand.Float64() * 5000,  // 0-5km
				ComputedAt:        time.Now(),
			})
		}

		if err := parcelFeatureRepo.BatchUpsert(ctx, features); err != nil {
			logger.Warn("Failed to create parcel features", zap.Error(err))
		} else {
			fmt.Fprintf(os.Stderr, "Created features for %d parcels\n", len(features))
		}
	}

	fmt.Fprintf(os.Stderr, "\n✓ Seed complete!\n")
	fmt.Fprintf(os.Stderr, "\nYou can now generate a token with:\n")
	fmt.Fprintf(os.Stderr, "  ./scout token --user-id %s --tenant-id %s --role analyst\n", userID.String(), tenantID.String())

	return nil
}

