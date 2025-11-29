-- Spatial indexes for parcels
CREATE INDEX idx_parcels_geom ON parcels USING GIST (geom);
CREATE INDEX idx_parcels_centroid ON parcels USING GIST (centroid);
CREATE INDEX idx_parcels_state_fips ON parcels (state_fips);
CREATE INDEX idx_parcels_tenant_id ON parcels (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_parcels_zoning_tags ON parcels USING GIN (zoning_tags);

-- Spatial indexes for layer tables
CREATE INDEX idx_rail_lines_geom ON rail_lines USING GIST (geom);
CREATE INDEX idx_rail_lines_tenant_id ON rail_lines (tenant_id) WHERE tenant_id IS NOT NULL;

CREATE INDEX idx_highways_geom ON highways USING GIST (geom);
CREATE INDEX idx_highways_tenant_id ON highways (tenant_id) WHERE tenant_id IS NOT NULL;

CREATE INDEX idx_airports_geom ON airports USING GIST (geom);
CREATE INDEX idx_airports_tenant_id ON airports (tenant_id) WHERE tenant_id IS NOT NULL;

CREATE INDEX idx_power_lines_geom ON power_lines USING GIST (geom);
CREATE INDEX idx_power_lines_tenant_id ON power_lines (tenant_id) WHERE tenant_id IS NOT NULL;

CREATE INDEX idx_substations_geom ON substations USING GIST (geom);
CREATE INDEX idx_substations_tenant_id ON substations (tenant_id) WHERE tenant_id IS NOT NULL;

CREATE INDEX idx_floodplains_geom ON floodplains USING GIST (geom);
CREATE INDEX idx_floodplains_tenant_id ON floodplains (tenant_id) WHERE tenant_id IS NOT NULL;

CREATE INDEX idx_wetlands_geom ON wetlands USING GIST (geom);
CREATE INDEX idx_wetlands_tenant_id ON wetlands (tenant_id) WHERE tenant_id IS NOT NULL;

CREATE INDEX idx_protected_land_geom ON protected_land USING GIST (geom);
CREATE INDEX idx_protected_land_tenant_id ON protected_land (tenant_id) WHERE tenant_id IS NOT NULL;

-- Indexes for parcel_features
CREATE INDEX idx_parcel_features_parcel_id ON parcel_features (parcel_id);
CREATE INDEX idx_parcel_features_version_id ON parcel_features (features_version_id);

-- Indexes for jobs (for polling)
CREATE INDEX idx_jobs_status_created ON jobs (status, created_at) WHERE status IN ('pending', 'processing');
CREATE INDEX idx_jobs_tenant_id ON jobs (tenant_id);

-- Indexes for exports
CREATE INDEX idx_exports_job_id ON exports (job_id);
CREATE INDEX idx_exports_tenant_id ON exports (tenant_id);

-- Indexes for saved searches
CREATE INDEX idx_saved_searches_tenant_id ON saved_searches (tenant_id);
CREATE INDEX idx_saved_searches_project_id ON saved_searches (project_id) WHERE project_id IS NOT NULL;

-- Indexes for projects
CREATE INDEX idx_projects_tenant_id ON projects (tenant_id);

-- Indexes for scoring profiles
CREATE INDEX idx_scoring_profiles_tenant_id ON scoring_profiles (tenant_id);

-- Indexes for memberships (for auth lookups)
CREATE INDEX idx_memberships_tenant_user ON memberships (tenant_id, user_id);
CREATE INDEX idx_memberships_user_id ON memberships (user_id);

-- Indexes for audit events
CREATE INDEX idx_audit_events_tenant_id ON audit_events (tenant_id);
CREATE INDEX idx_audit_events_created_at ON audit_events (created_at);

-- Indexes for data_runs
CREATE INDEX idx_data_runs_data_source_id ON data_runs (data_source_id);
CREATE INDEX idx_data_runs_status ON data_runs (status);



