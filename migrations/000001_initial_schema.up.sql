-- Enable PostGIS extension
CREATE EXTENSION IF NOT EXISTS postgis;

-- Tenancy & Auth tables
CREATE TABLE tenants (
    tenant_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
    user_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE memberships (
    membership_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('viewer', 'analyst', 'admin')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, user_id)
);

CREATE TABLE audit_events (
    audit_event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    actor_user_id UUID REFERENCES users(user_id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id UUID,
    meta_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Parcels table
CREATE TABLE parcels (
    parcel_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE SET NULL,
    apn TEXT NOT NULL,
    acres FLOAT NOT NULL,
    zoning_raw TEXT,
    zoning_tags TEXT[] DEFAULT '{}',
    jurisdiction TEXT,
    state_fips TEXT NOT NULL,
    geom GEOMETRY(POLYGON, 4326) NOT NULL,
    centroid GEOMETRY(POINT, 4326) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Layer tables (infrastructure and constraints)
CREATE TABLE rail_lines (
    feature_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE SET NULL,
    name TEXT,
    geom GEOMETRY(LINESTRING, 4326) NOT NULL,
    data_run_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE highways (
    feature_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE SET NULL,
    name TEXT,
    geom GEOMETRY(LINESTRING, 4326) NOT NULL,
    data_run_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE airports (
    feature_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE SET NULL,
    name TEXT,
    geom GEOMETRY(POINT, 4326) NOT NULL,
    data_run_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE power_lines (
    feature_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE SET NULL,
    name TEXT,
    geom GEOMETRY(LINESTRING, 4326) NOT NULL,
    data_run_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE substations (
    feature_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE SET NULL,
    name TEXT,
    geom GEOMETRY(POINT, 4326) NOT NULL,
    data_run_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE floodplains (
    feature_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE SET NULL,
    name TEXT,
    geom GEOMETRY(POLYGON, 4326) NOT NULL,
    data_run_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE wetlands (
    feature_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE SET NULL,
    name TEXT,
    geom GEOMETRY(POLYGON, 4326) NOT NULL,
    data_run_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE protected_land (
    feature_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE SET NULL,
    name TEXT,
    geom GEOMETRY(POLYGON, 4326) NOT NULL,
    data_run_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Feature store (precomputed)
CREATE TABLE parcel_features (
    parcel_id UUID PRIMARY KEY REFERENCES parcels(parcel_id) ON DELETE CASCADE,
    features_version_id UUID NOT NULL,
    in_floodplain BOOLEAN NOT NULL DEFAULT FALSE,
    in_wetlands BOOLEAN NOT NULL DEFAULT FALSE,
    in_protected_land BOOLEAN NOT NULL DEFAULT FALSE,
    dist_to_highway_m FLOAT,
    dist_to_rail_m FLOAT,
    dist_to_airport_m FLOAT,
    dist_to_power_line_m FLOAT,
    dist_to_substation_m FLOAT,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Data versioning / lineage
CREATE TABLE data_sources (
    data_source_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    provider TEXT,
    license_key TEXT,
    update_cadence TEXT,
    allowed_usage_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE data_runs (
    data_run_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    data_source_id UUID REFERENCES data_sources(data_source_id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    input_hash TEXT,
    row_count INTEGER,
    coverage_geom GEOMETRY(POLYGON, 4326),
    warnings_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE dataset_versions (
    dataset_version_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    layer_versions_json JSONB NOT NULL
);

-- Scoring profiles & saved work
CREATE TABLE scoring_profiles (
    scoring_profile_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    weights_json JSONB NOT NULL,
    thresholds_json JSONB,
    hard_constraints_json JSONB,
    created_by UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE projects (
    project_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE saved_searches (
    saved_search_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(project_id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    query_json JSONB NOT NULL,
    scoring_profile_id UUID REFERENCES scoring_profiles(scoring_profile_id) ON DELETE SET NULL,
    dataset_version_id UUID REFERENCES dataset_versions(dataset_version_id) ON DELETE SET NULL,
    created_by UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE favorites (
    favorite_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    parcel_id UUID NOT NULL REFERENCES parcels(parcel_id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, user_id, parcel_id)
);

-- Jobs & Exports
CREATE TABLE jobs (
    job_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('export', 'tile_build', 'feature_recompute')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')) DEFAULT 'pending',
    input_json JSONB,
    output_json JSONB,
    error_json JSONB,
    attempts INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE exports (
    export_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('pdf', 'csv', 'geojson')),
    object_key TEXT NOT NULL,
    content_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

