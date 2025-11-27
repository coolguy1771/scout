-- Private layers table for tenant-scoped layer uploads
CREATE TABLE private_layers (
    layer_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    layer_type TEXT NOT NULL CHECK (layer_type IN ('point', 'line', 'polygon', 'mixed')),
    file_name TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('uploading', 'processing', 'completed', 'failed')) DEFAULT 'uploading',
    data_run_id UUID REFERENCES data_runs(data_run_id) ON DELETE SET NULL,
    object_key TEXT, -- S3 key for the uploaded file
    error_message TEXT,
    created_by UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_private_layers_tenant_id ON private_layers (tenant_id);
CREATE INDEX idx_private_layers_status ON private_layers (status);
CREATE INDEX idx_private_layers_created_by ON private_layers (created_by);
CREATE INDEX idx_private_layers_created_at ON private_layers (created_at);

-- Update jobs table to include layer_upload job type
-- Note: PostgreSQL doesn't support modifying CHECK constraints directly
-- We need to drop and recreate the constraint
DO $$
BEGIN
    -- Drop the existing constraint
    ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
    
    -- Recreate with new job type
    ALTER TABLE jobs ADD CONSTRAINT jobs_type_check 
        CHECK (type IN ('export', 'tile_build', 'feature_recompute', 'layer_upload', 'search_sync'));
END $$;

-- Add RLS policies for private_layers
ALTER TABLE private_layers ENABLE ROW LEVEL SECURITY;

-- Policy: Users can view layers in their tenant
CREATE POLICY private_layers_tenant_select ON private_layers
    FOR SELECT
    USING (
        tenant_id IN (
            SELECT tenant_id FROM memberships 
            WHERE user_id = current_setting('app.current_user_id', true)::UUID
        )
    );

-- Policy: Only analysts and admins can insert layers
CREATE POLICY private_layers_tenant_insert ON private_layers
    FOR INSERT
    WITH CHECK (
        tenant_id IN (
            SELECT tenant_id FROM memberships 
            WHERE user_id = current_setting('app.current_user_id', true)::UUID
            AND role IN ('analyst', 'admin')
        )
    );

-- Policy: Only analysts and admins can update layers in their tenant
CREATE POLICY private_layers_tenant_update ON private_layers
    FOR UPDATE
    USING (
        tenant_id IN (
            SELECT tenant_id FROM memberships 
            WHERE user_id = current_setting('app.current_user_id', true)::UUID
            AND role IN ('analyst', 'admin')
        )
    );

-- Policy: Only admins can delete layers in their tenant
CREATE POLICY private_layers_tenant_delete ON private_layers
    FOR DELETE
    USING (
        tenant_id IN (
            SELECT tenant_id FROM memberships 
            WHERE user_id = current_setting('app.current_user_id', true)::UUID
            AND role = 'admin'
        )
    );

