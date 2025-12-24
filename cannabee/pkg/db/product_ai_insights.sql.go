// Manually generated. DO NOT EDIT.
// versions:
//   sqlc v1.30.0
// source: product_ai_insights.sql

package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ProductAIInsight represents the product_ai_insights table row
type ProductAIInsight struct {
	ID             uint64          `json:"id"`
	ProductID      uint64          `json:"product_id"`
	WhyThisProduct sql.NullString  `json:"why_this_product"`
	TopFeelings    json.RawMessage `json:"top_feelings"`
	KeyInfo        json.RawMessage `json:"key_info"`
	Ingredients    sql.NullString  `json:"ingredients"`
	CreatedAt      sql.NullTime    `json:"created_at"`
	UpdatedAt      sql.NullTime    `json:"updated_at"`
}

// KeyInfoJSON represents the structured key info for a product
type KeyInfoJSON struct {
	Flavor            string `json:"flavor,omitempty"`
	THCCBDContent     string `json:"thc_cbd_content,omitempty"`
	StrainType        string `json:"strain_type,omitempty"`
	TypicalTimeOfUse  string `json:"typical_time_of_use,omitempty"`
	ExpectedIntensity string `json:"expected_intensity,omitempty"`
	ConsumptionFormat string `json:"consumption_format,omitempty"`
}

const getProductAIInsightsByProductIDs = `-- name: GetProductAIInsightsByProductIDs :many
SELECT id, product_id, why_this_product, top_feelings, key_info, ingredients, created_at, updated_at
FROM product_ai_insights
WHERE product_id IN (/*SLICE:product_ids*/?)`

// GetProductAIInsightsByProductIDs retrieves AI insights for multiple products by their IDs
func (q *Queries) GetProductAIInsightsByProductIDs(ctx context.Context, productIDs []int64) ([]ProductAIInsight, error) {
	if len(productIDs) == 0 {
		return []ProductAIInsight{}, nil
	}

	query := getProductAIInsightsByProductIDs
	var queryParams []interface{}
	for _, id := range productIDs {
		queryParams = append(queryParams, id)
	}
	query = strings.Replace(query, "/*SLICE:product_ids*/?", strings.Repeat(",?", len(productIDs))[1:], 1)

	rows, err := q.db.QueryContext(ctx, query, queryParams...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ProductAIInsight
	for rows.Next() {
		var i ProductAIInsight
		if err := rows.Scan(
			&i.ID,
			&i.ProductID,
			&i.WhyThisProduct,
			&i.TopFeelings,
			&i.KeyInfo,
			&i.Ingredients,
			&i.CreatedAt,
			&i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const upsertProductAIInsight = `-- name: UpsertProductAIInsight :exec
INSERT INTO product_ai_insights (product_id, why_this_product, top_feelings, key_info, ingredients)
VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    why_this_product = VALUES(why_this_product),
    top_feelings = VALUES(top_feelings),
    key_info = VALUES(key_info),
    ingredients = VALUES(ingredients),
    updated_at = CURRENT_TIMESTAMP`

// UpsertProductAIInsightParams contains parameters for upserting a product AI insight
type UpsertProductAIInsightParams struct {
	ProductID      int64           `json:"product_id"`
	WhyThisProduct sql.NullString  `json:"why_this_product"`
	TopFeelings    json.RawMessage `json:"top_feelings"`
	KeyInfo        json.RawMessage `json:"key_info"`
	Ingredients    sql.NullString  `json:"ingredients"`
}

// UpsertProductAIInsight inserts or updates a product AI insight
func (q *Queries) UpsertProductAIInsight(ctx context.Context, arg UpsertProductAIInsightParams) error {
	_, err := q.db.ExecContext(ctx, upsertProductAIInsight,
		arg.ProductID,
		arg.WhyThisProduct,
		arg.TopFeelings,
		arg.KeyInfo,
		arg.Ingredients,
	)
	return err
}

const getProductAIInsightByProductID = `-- name: GetProductAIInsightByProductID :one
SELECT id, product_id, why_this_product, top_feelings, key_info, ingredients, created_at, updated_at
FROM product_ai_insights
WHERE product_id = ?`

// GetProductAIInsightByProductID retrieves a single product's AI insight
func (q *Queries) GetProductAIInsightByProductID(ctx context.Context, productID int64) (ProductAIInsight, error) {
	row := q.db.QueryRowContext(ctx, getProductAIInsightByProductID, productID)
	var i ProductAIInsight
	err := row.Scan(
		&i.ID,
		&i.ProductID,
		&i.WhyThisProduct,
		&i.TopFeelings,
		&i.KeyInfo,
		&i.Ingredients,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

// GetInsightsCacheExpiry returns the duration after which cached insights should be refreshed
func GetInsightsCacheExpiry() time.Duration {
	return 30 * 24 * time.Hour // 30 days
}
