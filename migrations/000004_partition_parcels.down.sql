-- Revert partitioning: convert back to non-partitioned table

-- Step 1: Create a new non-partitioned table
CREATE TABLE parcels_old (
    LIKE parcels INCLUDING ALL
);

-- Step 2: Copy all data from partitioned table to new table
INSERT INTO parcels_old
SELECT * FROM parcels;

-- Step 3: Drop the partitioned table (this will drop all partitions)
DROP TABLE parcels CASCADE;

-- Step 4: Rename the new table to the original name
ALTER TABLE parcels_old RENAME TO parcels;

-- Step 5: Recreate indexes
CREATE INDEX idx_parcels_geom ON parcels USING GIST (geom);
CREATE INDEX idx_parcels_centroid ON parcels USING GIST (centroid);
CREATE INDEX idx_parcels_state_fips ON parcels (state_fips);
CREATE INDEX idx_parcels_tenant_id ON parcels (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_parcels_zoning_tags ON parcels USING GIN (zoning_tags);

-- Step 6: Recreate foreign key constraint
ALTER TABLE parcel_features 
ADD CONSTRAINT parcel_features_parcel_id_fkey 
FOREIGN KEY (parcel_id) REFERENCES parcels(parcel_id) ON DELETE CASCADE;

