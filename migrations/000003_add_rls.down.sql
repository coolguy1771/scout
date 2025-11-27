-- Drop RLS policies
DROP POLICY IF EXISTS audit_events_insert ON audit_events;
DROP POLICY IF EXISTS audit_events_select ON audit_events;
DROP POLICY IF EXISTS favorites_delete ON favorites;
DROP POLICY IF EXISTS favorites_insert ON favorites;
DROP POLICY IF EXISTS favorites_select ON favorites;
DROP POLICY IF EXISTS exports_insert ON exports;
DROP POLICY IF EXISTS exports_select ON exports;
DROP POLICY IF EXISTS jobs_update ON jobs;
DROP POLICY IF EXISTS jobs_insert ON jobs;
DROP POLICY IF EXISTS jobs_select ON jobs;
DROP POLICY IF EXISTS scoring_profiles_delete ON scoring_profiles;
DROP POLICY IF EXISTS scoring_profiles_update ON scoring_profiles;
DROP POLICY IF EXISTS scoring_profiles_insert ON scoring_profiles;
DROP POLICY IF EXISTS scoring_profiles_select ON scoring_profiles;
DROP POLICY IF EXISTS saved_searches_delete ON saved_searches;
DROP POLICY IF EXISTS saved_searches_update ON saved_searches;
DROP POLICY IF EXISTS saved_searches_insert ON saved_searches;
DROP POLICY IF EXISTS saved_searches_select ON saved_searches;
DROP POLICY IF EXISTS projects_delete ON projects;
DROP POLICY IF EXISTS projects_update ON projects;
DROP POLICY IF EXISTS projects_insert ON projects;
DROP POLICY IF EXISTS projects_select ON projects;
DROP POLICY IF EXISTS protected_land_select ON protected_land;
DROP POLICY IF EXISTS wetlands_select ON wetlands;
DROP POLICY IF EXISTS floodplains_select ON floodplains;
DROP POLICY IF EXISTS substations_select ON substations;
DROP POLICY IF EXISTS power_lines_select ON power_lines;
DROP POLICY IF EXISTS airports_select ON airports;
DROP POLICY IF EXISTS highways_select ON highways;
DROP POLICY IF EXISTS rail_lines_select ON rail_lines;
DROP POLICY IF EXISTS parcels_delete ON parcels;
DROP POLICY IF EXISTS parcels_update ON parcels;
DROP POLICY IF EXISTS parcels_insert ON parcels;
DROP POLICY IF EXISTS parcels_select ON parcels;

-- Disable RLS
ALTER TABLE audit_events DISABLE ROW LEVEL SECURITY;
ALTER TABLE favorites DISABLE ROW LEVEL SECURITY;
ALTER TABLE exports DISABLE ROW LEVEL SECURITY;
ALTER TABLE jobs DISABLE ROW LEVEL SECURITY;
ALTER TABLE scoring_profiles DISABLE ROW LEVEL SECURITY;
ALTER TABLE saved_searches DISABLE ROW LEVEL SECURITY;
ALTER TABLE projects DISABLE ROW LEVEL SECURITY;
ALTER TABLE protected_land DISABLE ROW LEVEL SECURITY;
ALTER TABLE wetlands DISABLE ROW LEVEL SECURITY;
ALTER TABLE floodplains DISABLE ROW LEVEL SECURITY;
ALTER TABLE substations DISABLE ROW LEVEL SECURITY;
ALTER TABLE power_lines DISABLE ROW LEVEL SECURITY;
ALTER TABLE airports DISABLE ROW LEVEL SECURITY;
ALTER TABLE highways DISABLE ROW LEVEL SECURITY;
ALTER TABLE rail_lines DISABLE ROW LEVEL SECURITY;
ALTER TABLE parcels DISABLE ROW LEVEL SECURITY;

-- Drop helper functions
DROP FUNCTION IF EXISTS current_user_id();
DROP FUNCTION IF EXISTS current_tenant_id();

