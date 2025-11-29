-- Enable Row Level Security on tenant-scoped tables
ALTER TABLE parcels ENABLE ROW LEVEL SECURITY;
ALTER TABLE rail_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE highways ENABLE ROW LEVEL SECURITY;
ALTER TABLE airports ENABLE ROW LEVEL SECURITY;
ALTER TABLE power_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE substations ENABLE ROW LEVEL SECURITY;
ALTER TABLE floodplains ENABLE ROW LEVEL SECURITY;
ALTER TABLE wetlands ENABLE ROW LEVEL SECURITY;
ALTER TABLE protected_land ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE saved_searches ENABLE ROW LEVEL SECURITY;
ALTER TABLE scoring_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE exports ENABLE ROW LEVEL SECURITY;
ALTER TABLE favorites ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;

-- Create function to get current tenant_id from JWT claim
-- This will be set by the application via SET LOCAL
CREATE OR REPLACE FUNCTION current_tenant_id() RETURNS UUID AS $$
    SELECT current_setting('app.current_tenant_id', TRUE)::UUID;
$$ LANGUAGE sql STABLE;

-- Create function to get current user_id from JWT claim
CREATE OR REPLACE FUNCTION current_user_id() RETURNS UUID AS $$
    SELECT current_setting('app.current_user_id', TRUE)::UUID;
$$ LANGUAGE sql STABLE;

-- RLS Policies for parcels
-- Users can see parcels where tenant_id matches their tenant OR tenant_id IS NULL (public parcels)
CREATE POLICY parcels_select ON parcels
    FOR SELECT
    USING (
        tenant_id = current_tenant_id() OR tenant_id IS NULL
    );

CREATE POLICY parcels_insert ON parcels
    FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY parcels_update ON parcels
    FOR UPDATE
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL)
    WITH CHECK (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY parcels_delete ON parcels
    FOR DELETE
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

-- RLS Policies for layer tables (similar pattern)
CREATE POLICY rail_lines_select ON rail_lines
    FOR SELECT
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY highways_select ON highways
    FOR SELECT
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY airports_select ON airports
    FOR SELECT
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY power_lines_select ON power_lines
    FOR SELECT
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY substations_select ON substations
    FOR SELECT
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY floodplains_select ON floodplains
    FOR SELECT
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY wetlands_select ON wetlands
    FOR SELECT
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY protected_land_select ON protected_land
    FOR SELECT
    USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

-- RLS Policies for projects
CREATE POLICY projects_select ON projects
    FOR SELECT
    USING (tenant_id = current_tenant_id());

CREATE POLICY projects_insert ON projects
    FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id());

CREATE POLICY projects_update ON projects
    FOR UPDATE
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

CREATE POLICY projects_delete ON projects
    FOR DELETE
    USING (tenant_id = current_tenant_id());

-- RLS Policies for saved_searches
CREATE POLICY saved_searches_select ON saved_searches
    FOR SELECT
    USING (tenant_id = current_tenant_id());

CREATE POLICY saved_searches_insert ON saved_searches
    FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id());

CREATE POLICY saved_searches_update ON saved_searches
    FOR UPDATE
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

CREATE POLICY saved_searches_delete ON saved_searches
    FOR DELETE
    USING (tenant_id = current_tenant_id());

-- RLS Policies for scoring_profiles
CREATE POLICY scoring_profiles_select ON scoring_profiles
    FOR SELECT
    USING (tenant_id = current_tenant_id());

CREATE POLICY scoring_profiles_insert ON scoring_profiles
    FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id());

CREATE POLICY scoring_profiles_update ON scoring_profiles
    FOR UPDATE
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

CREATE POLICY scoring_profiles_delete ON scoring_profiles
    FOR DELETE
    USING (tenant_id = current_tenant_id());

-- RLS Policies for jobs
CREATE POLICY jobs_select ON jobs
    FOR SELECT
    USING (tenant_id = current_tenant_id());

CREATE POLICY jobs_insert ON jobs
    FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id());

CREATE POLICY jobs_update ON jobs
    FOR UPDATE
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- RLS Policies for exports
CREATE POLICY exports_select ON exports
    FOR SELECT
    USING (tenant_id = current_tenant_id());

CREATE POLICY exports_insert ON exports
    FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id());

-- RLS Policies for favorites
CREATE POLICY favorites_select ON favorites
    FOR SELECT
    USING (tenant_id = current_tenant_id() AND user_id = current_user_id());

CREATE POLICY favorites_insert ON favorites
    FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id() AND user_id = current_user_id());

CREATE POLICY favorites_delete ON favorites
    FOR DELETE
    USING (tenant_id = current_tenant_id() AND user_id = current_user_id());

-- RLS Policies for audit_events
CREATE POLICY audit_events_select ON audit_events
    FOR SELECT
    USING (tenant_id = current_tenant_id());

CREATE POLICY audit_events_insert ON audit_events
    FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id());



