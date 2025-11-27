-- Drop private layers table and related objects

DROP POLICY IF EXISTS private_layers_tenant_delete ON private_layers;
DROP POLICY IF EXISTS private_layers_tenant_update ON private_layers;
DROP POLICY IF EXISTS private_layers_tenant_insert ON private_layers;
DROP POLICY IF EXISTS private_layers_tenant_select ON private_layers;

DROP TABLE IF EXISTS private_layers;

