-- name: GetProductsPrices :many
-- Get the price of products from item_preco with a given codigo_prazo.
-- Later we'll filter by the most recent alteracao_preco. If there are two with the same 
-- alteracao_preco, we'll send a warning to the admin through email. 
SELECT codigo_item, codigo_unidade, preco, alteracao_preco FROM item_preco WHERE codigo_item = ANY(sqlc.arg(codigos_itens)::pg_catalog.numeric[]) and codigo_prazo == $1 ORDER BY codigo_item, alteracao_preco DESC;

-- name: GetNotificationEvent :one
SELECT event_type, codigo_item, date_sent FROM vn_last_notification_event WHERE event_type == $1 and codigo_item == $2;

-- name: RegisterNotificationEvent :exec
INSERT INTO vn_last_notification_event (event_type, codigo_item, date_sent) VALUES ($1, $2, now());

-- name: ClearNotificationEvents :exec
-- ClearNotificationEvents deletes old notification exem.
DELETE FROM vn_last_notification_event WHERE date_sent < now() - interval '1 hour' * sqlc.arg(number_of_hours)::integer;

-- name: UpdatePriceWatcher :exec
-- UpdatePriceWatcher updates the last hash and update time of the prices.
INSERT INTO vn_price_update_watcher (last_update, prices_hash) VALUES (now(), $1);

-- name: GetPriceWatcher :one
-- GetPriceWatcher retrieves the last hash and update time of the prices.
SELECT last_update, prices_hash FROM vn_price_update_watcher ORDER BY last_update DESC LIMIT 1;

-- name: GetAllProducts :many
-- GetAllProducts is used to calculate the hash of the products and prices. Always sorted in a predictable order.
-- This is used to check if the prices have changed.
SELECT codigo_item, preco FROM item_preco WHERE codigo_prazo == $1 ORDER BY codigo_item, codigo_unidade DESC;

-- name: GetMostRecentUpdate :one
-- GetMostRecentUpdate retrieves the most recent update of the prices.
SELECT alteracao_preco FROM item_preco ORDER BY alteracao_preco DESC LIMIT 1;