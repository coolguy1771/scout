-- Partition parcels table by state_fips
-- This migration converts the parcels table to a partitioned table

-- Step 1: Create the partitioned parent table
CREATE TABLE parcels_new (
    LIKE parcels INCLUDING ALL
) PARTITION BY LIST (state_fips);

-- Step 2: Create a default partition for unknown states
CREATE TABLE parcels_default PARTITION OF parcels_new
    FOR VALUES IN (DEFAULT);

-- Step 3: Create partitions for existing states
-- First, get distinct state_fips and create partitions
DO $$
DECLARE
    state_code TEXT;
BEGIN
    FOR state_code IN SELECT DISTINCT state_fips FROM parcels WHERE state_fips IS NOT NULL
    LOOP
        EXECUTE format('CREATE TABLE parcels_%s PARTITION OF parcels_new FOR VALUES IN (%L)',
            replace(state_code, '-', '_'), state_code);
    END LOOP;
END $$;

-- Step 4: Copy data from old table to new partitioned table
INSERT INTO parcels_new
SELECT * FROM parcels;

-- Step 5: Drop the old table
DROP TABLE parcels CASCADE;

-- Step 6: Rename the new table to the original name
ALTER TABLE parcels_new RENAME TO parcels;

-- Step 7: Recreate indexes on the parent table (they will be inherited by partitions)
-- Note: GIST indexes need to be created on each partition separately
CREATE INDEX idx_parcels_geom ON parcels USING GIST (geom);
CREATE INDEX idx_parcels_centroid ON parcels USING GIST (centroid);
CREATE INDEX idx_parcels_state_fips ON parcels (state_fips);
CREATE INDEX idx_parcels_tenant_id ON parcels (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_parcels_zoning_tags ON parcels USING GIN (zoning_tags);

-- Step 8: Recreate indexes on existing partitions
DO $$
DECLARE
    partition_name TEXT;
BEGIN
    FOR partition_name IN 
        SELECT tablename FROM pg_tables 
        WHERE schemaname = 'public' 
        AND tablename LIKE 'parcels_%' 
        AND tablename != 'parcels_default'
    LOOP
        EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%s_geom ON %I USING GIST (geom)',
            partition_name, partition_name);
        EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%s_centroid ON %I USING GIST (centroid)',
            partition_name, partition_name);
    END LOOP;
END $$;

-- Step 9: Update foreign key constraints
-- The parcel_features table references parcels, so we need to ensure the constraint is maintained
-- (This should already be in place from the original schema, but we verify)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'parcel_features_parcel_id_fkey'
    ) THEN
        ALTER TABLE parcel_features 
        ADD CONSTRAINT parcel_features_parcel_id_fkey 
        FOREIGN KEY (parcel_id) REFERENCES parcels(parcel_id) ON DELETE CASCADE;
    END IF;
END $$;

