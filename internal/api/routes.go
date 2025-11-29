package api

import (
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
)

// RegisterRoutes registers all API routes
func RegisterRoutes(r chi.Router, db *database.DB, logger *zap.Logger, cfg *config.Config) {
	// Initialize handlers
	parcelHandler := NewParcelHandler(db, logger, cfg)
	searchHandler := NewSearchHandler(db, logger, cfg)
	projectHandler := NewProjectHandler(db, logger, cfg)
	exportHandler := NewExportHandler(db, logger, cfg)
	scoringProfileHandler := NewScoringProfileHandler(db, logger, cfg)

	// Parcel routes
	r.Route("/parcels", func(r chi.Router) {
		r.Get("/{parcelId}", parcelHandler.GetParcel)
		r.Get("/{parcelId}/nearby", parcelHandler.GetNearby)
	})

	// Search routes
	r.Route("/suitability", func(r chi.Router) {
		r.Post("/search", searchHandler.Search)
	})

	// Project routes
	r.Route("/projects", func(r chi.Router) {
		r.Post("/", projectHandler.CreateProject)
		r.Get("/", projectHandler.ListProjects)
		r.Get("/{projectId}", projectHandler.GetProject)
		r.Put("/{projectId}", projectHandler.UpdateProject)
		r.Delete("/{projectId}", projectHandler.DeleteProject)
	})

	// Saved search routes
	r.Route("/savedSearches", func(r chi.Router) {
		r.Post("/", projectHandler.CreateSavedSearch)
		r.Get("/", projectHandler.ListSavedSearches)
		r.Get("/{savedSearchId}", projectHandler.GetSavedSearch)
		r.Put("/{savedSearchId}", projectHandler.UpdateSavedSearch)
		r.Get("/{savedSearchId}/run", projectHandler.RunSavedSearch)
		r.Delete("/{savedSearchId}", projectHandler.DeleteSavedSearch)
	})

	// Export routes
	r.Route("/exports", func(r chi.Router) {
		r.Post("/", exportHandler.CreateExport)
		r.Get("/", exportHandler.ListExports)
		r.Get("/{exportId}", exportHandler.GetExport)
	})

	// Job routes
	r.Route("/jobs", func(r chi.Router) {
		r.Get("/", exportHandler.ListJobs)
		r.Get("/{jobId}", exportHandler.GetJob)
	})

	// Layer routes
	layerHandler := NewLayerHandler(db, logger, cfg)
	r.Route("/layers", func(r chi.Router) {
		r.Post("/upload", layerHandler.UploadLayer)
		r.Get("/", layerHandler.ListLayers)
		r.Get("/{layerId}", layerHandler.GetLayer)
		r.Put("/{layerId}", layerHandler.UpdateLayer)
		r.Delete("/{layerId}", layerHandler.DeleteLayer)
	})

	// Scoring profile routes
	r.Route("/scoringProfiles", func(r chi.Router) {
		r.Post("/", scoringProfileHandler.CreateScoringProfile)
		r.Get("/", scoringProfileHandler.ListScoringProfiles)
		r.Get("/{scoringProfileId}", scoringProfileHandler.GetScoringProfile)
		r.Put("/{scoringProfileId}", scoringProfileHandler.UpdateScoringProfile)
		r.Delete("/{scoringProfileId}", scoringProfileHandler.DeleteScoringProfile)
	})
}

// RegisterTileRoutes registers tile serving routes
func RegisterTileRoutes(r chi.Router, db *database.DB, logger *zap.Logger, cfg *config.Config) {
	tileHandler := NewTileHandler(db, logger, cfg)
	r.Get("/{layer}/{z}/{x}/{y}.pbf", tileHandler.GetTile)
}
