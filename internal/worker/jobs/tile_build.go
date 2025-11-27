package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
	"github.com/coolguy1771/scout/internal/storage"
)

type TileBuildJob struct {
	jobRepo     *repository.JobRepository
	tileStorage *storage.TileStorage
	logger      *zap.Logger
}

func NewTileBuildJob(jobRepo *repository.JobRepository, tileStorage *storage.TileStorage, logger *zap.Logger) *TileBuildJob {
	return &TileBuildJob{
		jobRepo:     jobRepo,
		tileStorage: tileStorage,
		logger:      logger,
	}
}

type TileBuildInput struct {
	Layer      string  `json:"layer"`
	TenantID   *string `json:"tenantId,omitempty"`
	ZoomLevels []int   `json:"zoomLevels,omitempty"`
}

func (j *TileBuildJob) Process(ctx context.Context, job *models.Job) error {
	// Parse input
	var input TileBuildInput
	if err := json.Unmarshal([]byte(job.InputJSON), &input); err != nil {
		return fmt.Errorf("failed to parse tile build input: %w", err)
	}

	j.logger.Info("Tile build job started",
		zap.String("layer", input.Layer),
		zap.String("jobId", job.JobID.String()),
		zap.Any("zoomLevels", input.ZoomLevels))

	// Check if tile storage is available
	if j.tileStorage == nil {
		return fmt.Errorf("tile storage not configured")
	}

	// Get database connection
	db := j.jobRepo.GetDB()

	// Map layer name to table name
	tableName, err := j.getTableName(input.Layer)
	if err != nil {
		return fmt.Errorf("invalid layer name: %w", err)
	}

	// Determine zoom levels (default: 0-10 if not specified)
	// Higher zoom levels generate exponentially more tiles (zoom 10 = 1M tiles, zoom 14 = 268M tiles)
	// Only generate tiles that intersect with actual data to avoid processing empty tiles
	zoomLevels := input.ZoomLevels
	if len(zoomLevels) == 0 {
		zoomLevels = make([]int, 11)
		for i := 0; i <= 10; i++ {
			zoomLevels[i] = i
		}
	}

	// Convert tenantID string to UUID if provided
	var tenantUUID *uuid.UUID
	if input.TenantID != nil {
		parsed, err := uuid.Parse(*input.TenantID)
		if err != nil {
			return fmt.Errorf("invalid tenant ID: %w", err)
		}
		tenantUUID = &parsed
	}

	// Get the extent of data to only generate tiles within bounds
	dataExtent, err := j.getDataExtent(ctx, db, tableName, tenantUUID)
	if err != nil {
		j.logger.Warn("Failed to get data extent, will generate all tiles",
			zap.Error(err))
		dataExtent = nil // Will generate all tiles if extent unavailable
	} else if dataExtent != nil {
		j.logger.Info("Data extent found, will only generate tiles within bounds",
			zap.Float64("minLon", dataExtent.MinLon),
			zap.Float64("maxLon", dataExtent.MaxLon),
			zap.Float64("minLat", dataExtent.MinLat),
			zap.Float64("maxLat", dataExtent.MaxLat))
	}

	// Generate tiles for each zoom level
	tilesGenerated := 0
	tilesSkipped := 0
	errors := []string{}

	for _, z := range zoomLevels {
		if z < 0 || z > 18 {
			j.logger.Warn("Skipping invalid zoom level", zap.Int("zoom", z))
			continue
		}

		// Calculate tile range for this zoom level
		var minX, maxX, minY, maxY int
		if dataExtent != nil {
			// Only generate tiles within data extent
			minX, minY = lonLatToTile(z, dataExtent.MinLon, dataExtent.MaxLat)
			maxX, maxY = lonLatToTile(z, dataExtent.MaxLon, dataExtent.MinLat)
			// Add padding to ensure we don't miss edge tiles
			minX = max(0, minX-1)
			minY = max(0, minY-1)
			tileCount := int(math.Pow(2, float64(z)))
			maxX = min(tileCount-1, maxX+1)
			maxY = min(tileCount-1, maxY+1)
		} else {
			// Generate all tiles if no extent available
			tileCount := int(math.Pow(2, float64(z)))
			minX, minY = 0, 0
			maxX, maxY = tileCount-1, tileCount-1
		}

		j.logger.Info("Processing zoom level",
			zap.Int("zoom", z),
			zap.Int("tileRangeX", maxX-minX+1),
			zap.Int("tileRangeY", maxY-minY+1))

		// Generate tiles for this zoom level within bounds
		for x := minX; x <= maxX; x++ {
			for y := minY; y <= maxY; y++ {
				// Check if tile already exists
				exists, err := j.tileStorage.TileExists(ctx, input.Layer, z, x, y, input.TenantID)
				if err != nil {
					j.logger.Warn("Failed to check tile existence",
						zap.Int("z", z), zap.Int("x", x), zap.Int("y", y),
						zap.Error(err))
					// Continue anyway, will try to generate
				}
				if exists {
					tilesSkipped++
					continue
				}

				// Generate tile
				tileData, err := j.generateTile(ctx, db, tableName, z, x, y, tenantUUID)
				if err != nil {
					errorMsg := fmt.Sprintf("Failed to generate tile z=%d/x=%d/y=%d: %v", z, x, y, err)
					errors = append(errors, errorMsg)
					j.logger.Warn("Failed to generate tile",
						zap.Int("z", z), zap.Int("x", x), zap.Int("y", y),
						zap.Error(err))
					continue
				}

				// Skip empty tiles
				if len(tileData) == 0 {
					tilesSkipped++
					continue
				}

				// Upload tile to S3
				if err := j.tileStorage.UploadTile(ctx, input.Layer, z, x, y, input.TenantID, tileData); err != nil {
					errorMsg := fmt.Sprintf("Failed to upload tile z=%d/x=%d/y=%d: %v", z, x, y, err)
					errors = append(errors, errorMsg)
					j.logger.Warn("Failed to upload tile",
						zap.Int("z", z), zap.Int("x", x), zap.Int("y", y),
						zap.Error(err))
					continue
				}

				tilesGenerated++
			}
		}

		j.logger.Info("Completed zoom level",
			zap.Int("zoom", z),
			zap.Int("tilesGenerated", tilesGenerated),
			zap.Int("tilesSkipped", tilesSkipped))
	}

	// Prepare output
	output := map[string]interface{}{
		"message":        "Tile build completed",
		"layer":          input.Layer,
		"tilesGenerated": tilesGenerated,
		"tilesSkipped":   tilesSkipped,
		"zoomLevels":     zoomLevels,
	}
	if len(errors) > 0 {
		output["errors"] = errors
		output["errorCount"] = len(errors)
	}

	outputJSON, _ := json.Marshal(output)
	status := "completed"
	if len(errors) > 0 && tilesGenerated == 0 {
		status = "failed"
	}

	return j.jobRepo.UpdateStatus(ctx, job.JobID, status, string(outputJSON), nil)
}

// getTableName maps layer names to database table names
func (j *TileBuildJob) getTableName(layer string) (string, error) {
	// Normalize layer name (handle both snake_case and kebab-case)
	normalized := strings.ToLower(strings.ReplaceAll(layer, "-", "_"))

	// Map of valid layer names to table names
	validLayers := map[string]string{
		"rail_lines":     "rail_lines",
		"rail-lines":     "rail_lines",
		"highways":       "highways",
		"airports":       "airports",
		"power_lines":    "power_lines",
		"power-lines":    "power_lines",
		"substations":    "substations",
		"floodplains":    "floodplains",
		"wetlands":       "wetlands",
		"protected_land": "protected_land",
		"protected-land": "protected_land",
		"parcels":        "parcels",
	}

	tableName, ok := validLayers[normalized]
	if !ok {
		return "", fmt.Errorf("unknown layer: %s (valid layers: %v)", layer, getKeys(validLayers))
	}

	return tableName, nil
}

// generateTile generates a vector tile using PostGIS ST_AsMVT
func (j *TileBuildJob) generateTile(ctx context.Context, db *sqlx.DB, tableName string, z, x, y int, tenantID *uuid.UUID) ([]byte, error) {
	// Calculate tile bounding box in Web Mercator (EPSG:3857)
	bbox := tileToBBox(z, x, y)

	// Determine ID column name (parcels uses parcel_id, others use feature_id)
	idColumn := "feature_id"
	if tableName == "parcels" {
		idColumn = "parcel_id"
	}

	// Determine name column (parcels uses apn, others use name)
	nameColumn := "name"
	if tableName == "parcels" {
		nameColumn = "apn"
	}

	// Build query with tenant filter if provided
	var query string
	var args []interface{}

	if tenantID != nil {
		query = fmt.Sprintf(`
			SELECT ST_AsMVT(q, '%s', 4096, 'geom') as mvt
			FROM (
				SELECT 
					%s::text as id,
					%s as name,
					ST_AsMVTGeom(
						geom,
						ST_MakeEnvelope($1, $2, $3, $4, 3857),
						4096,
						256,
						true
					) AS geom
				FROM %s
				WHERE tenant_id = $5
				  AND geom && ST_Transform(ST_MakeEnvelope($1, $2, $3, $4, 3857), 4326)
			) q
			WHERE geom IS NOT NULL
		`, tableName, idColumn, nameColumn, tableName)
		args = []interface{}{bbox.MinX, bbox.MinY, bbox.MaxX, bbox.MaxY, *tenantID}
	} else {
		query = fmt.Sprintf(`
			SELECT ST_AsMVT(q, '%s', 4096, 'geom') as mvt
			FROM (
				SELECT 
					%s::text as id,
					%s as name,
					ST_AsMVTGeom(
						geom,
						ST_MakeEnvelope($1, $2, $3, $4, 3857),
						4096,
						256,
						true
					) AS geom
				FROM %s
				WHERE tenant_id IS NULL
				  AND geom && ST_Transform(ST_MakeEnvelope($1, $2, $3, $4, 3857), 4326)
			) q
			WHERE geom IS NOT NULL
		`, tableName, idColumn, nameColumn, tableName)
		args = []interface{}{bbox.MinX, bbox.MinY, bbox.MaxX, bbox.MaxY}
	}

	var tileData []byte
	err := db.GetContext(ctx, &tileData, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tile: %w", err)
	}

	return tileData, nil
}

// BBox represents a bounding box in Web Mercator (EPSG:3857)
type BBox struct {
	MinX, MinY, MaxX, MaxY float64
}

// tileToBBox converts tile coordinates (z, x, y) to Web Mercator bounding box
func tileToBBox(z, x, y int) BBox {
	n := math.Pow(2, float64(z))
	minX := float64(x)/n*360.0 - 180.0
	maxX := float64(x+1)/n*360.0 - 180.0
	minY := math.Atan(math.Sinh(math.Pi * (1 - 2*float64(y+1)/n)))
	maxY := math.Atan(math.Sinh(math.Pi * (1 - 2*float64(y)/n)))

	// Convert to Web Mercator (EPSG:3857)
	// Web Mercator uses meters, so we need to convert from degrees
	earthRadius := 6378137.0 // meters
	minXMerc := minX * math.Pi / 180.0 * earthRadius
	maxXMerc := maxX * math.Pi / 180.0 * earthRadius
	minYMerc := math.Log(math.Tan((90.0+minY*180.0/math.Pi)*math.Pi/360.0)) / (math.Pi / 180.0) * earthRadius
	maxYMerc := math.Log(math.Tan((90.0+maxY*180.0/math.Pi)*math.Pi/360.0)) / (math.Pi / 180.0) * earthRadius

	return BBox{
		MinX: minXMerc,
		MinY: minYMerc,
		MaxX: maxXMerc,
		MaxY: maxYMerc,
	}
}

// getKeys returns the keys of a map as a slice
func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// DataExtent represents the geographic extent of data in WGS84 (lon/lat)
type DataExtent struct {
	MinLon, MinLat, MaxLon, MaxLat float64
}

// getDataExtent gets the bounding box of all features in the layer
func (j *TileBuildJob) getDataExtent(ctx context.Context, db *sqlx.DB, tableName string, tenantID *uuid.UUID) (*DataExtent, error) {
	var query string
	var args []interface{}

	if tenantID != nil {
		query = fmt.Sprintf(`
			SELECT 
				ST_XMin(ST_Extent(geom)) as min_lon,
				ST_YMin(ST_Extent(geom)) as min_lat,
				ST_XMax(ST_Extent(geom)) as max_lon,
				ST_YMax(ST_Extent(geom)) as max_lat
			FROM %s
			WHERE tenant_id = $1
		`, tableName)
		args = []interface{}{*tenantID}
	} else {
		query = fmt.Sprintf(`
			SELECT 
				ST_XMin(ST_Extent(geom)) as min_lon,
				ST_YMin(ST_Extent(geom)) as min_lat,
				ST_XMax(ST_Extent(geom)) as max_lon,
				ST_YMax(ST_Extent(geom)) as max_lat
			FROM %s
			WHERE tenant_id IS NULL
		`, tableName)
		args = []interface{}{}
	}

	var extent struct {
		MinLon sql.NullFloat64 `db:"min_lon"`
		MinLat sql.NullFloat64 `db:"min_lat"`
		MaxLon sql.NullFloat64 `db:"max_lon"`
		MaxLat sql.NullFloat64 `db:"max_lat"`
	}
	err := db.GetContext(ctx, &extent, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get data extent: %w", err)
	}

	// Check if extent is valid
	if !extent.MinLon.Valid || !extent.MaxLon.Valid || !extent.MinLat.Valid || !extent.MaxLat.Valid {
		return nil, fmt.Errorf("no data found in layer")
	}

	return &DataExtent{
		MinLon: extent.MinLon.Float64,
		MinLat: extent.MinLat.Float64,
		MaxLon: extent.MaxLon.Float64,
		MaxLat: extent.MaxLat.Float64,
	}, nil
}

// lonLatToTile converts longitude/latitude to tile coordinates at a given zoom level
func lonLatToTile(zoom int, lon, lat float64) (x, y int) {
	n := math.Pow(2, float64(zoom))
	x = int((lon + 180.0) / 360.0 * n)
	latRad := lat * math.Pi / 180.0
	y = int((1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n)
	return x, y
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
