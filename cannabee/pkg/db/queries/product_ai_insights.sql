-- Get product AI insights by product IDs
-- name: GetProductAIInsightsByProductIDs :many
SELECT id, product_id, why_this_product, top_feelings, key_info, ingredients, created_at, updated_at
FROM product_ai_insights
WHERE product_id IN (sqlc.slice('product_ids'));

-- Upsert product AI insight (insert or update if exists)
-- name: UpsertProductAIInsight :exec
INSERT INTO product_ai_insights (product_id, why_this_product, top_feelings, key_info, ingredients)
VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    why_this_product = VALUES(why_this_product),
    top_feelings = VALUES(top_feelings),
    key_info = VALUES(key_info),
    ingredients = VALUES(ingredients),
    updated_at = CURRENT_TIMESTAMP;

-- Get single product AI insight by product ID
-- name: GetProductAIInsightByProductID :one
SELECT id, product_id, why_this_product, top_feelings, key_info, ingredients, created_at, updated_at
FROM product_ai_insights
WHERE product_id = ?;

