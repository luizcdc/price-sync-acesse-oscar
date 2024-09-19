-- name: GetAllProductsThatNeedUpdate :many
SELECT DISTINCT partner_sku FROM partner_stockrecord WHERE partner_id = $1 AND date_updated <= $2;

-- name: UpdateProductPrice :batchexec
-- Update products from the values in the item_preco table
UPDATE partner_stockrecord SET price = @price::pg_catalog.numeric, date_updated = now() WHERE partner_id = $1 AND partner_sku = $2;
