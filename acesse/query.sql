-- name: GetProducts :many
-- Get the price of products from item_preco with a given codigo_prazo.
-- Later we'll filter by the most recent alteracao_preco. If there are two with the same 
-- alteracao_preco, we'll send a warning to the admin through email. 
SELECT codigo_item, codigo_unidade, preco, alteracao_preco FROM item_preco WHERE codigo_item = ANY($1::int[]) and codigo_prazo == $2;

-- name: getNotificationEvent :one
SELECT event_type, codigo_item, date_sent FROM vn_last_notification_event WHERE event_type == $1 and codigo_item == $2;

-- name: registerNotificationEvent :exec
INSERT INTO vn_last_notification_event (event_type, codigo_item, date_sent) VALUES ($1, $2, now());

-- name: clearNotificationEvents :exec
DELETE FROM vn_last_notification_event WHERE date_sent < now() - interval '1 hour' * $1;

-- name: updatePriceWatcher :exec
INSERT INTO vn_price_update_watcher (last_update, prices_hash) VALUES (now(), $1);

-- name: getPriceWatcher :one
SELECT last_update, prices_hash FROM vn_price_update_watcher ORDER BY last_update DESC LIMIT 1;

-- name: getAllProducts :many
SELECT codigo_item, preco, alteracao_preco FROM item_preco WHERE codigo_prazo == $1 ORDER BY alteracao_preco, codigo_item, preco DESC;

