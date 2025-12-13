-- Get product listings by product IDs (existing query)
-- name: GetProductListingsByProductIDs :many
SELECT listing_id, price, discounted_price, inventory_count, listing_url, image_urls, thc_percentage, cbd_percentage, description, listing_date, retailer_id, product_id, created_by, updated_by, created_at, updated_at, is_active
FROM product_listings_prd
WHERE product_id IN (sqlc.slice('product_ids'))
AND is_active = TRUE
ORDER BY product_id, price ASC;

-- NEW: Get product listings within radius by product IDs (Optimized with bounding box pre-filtering)
-- name: GetProductListingsWithinRadiusByProductIDs :many
SELECT 
    pl.listing_id, 
    pl.price, 
    pl.discounted_price, 
    pl.inventory_count, 
    pl.listing_url, 
    pl.image_urls, 
    pl.thc_percentage, 
    pl.cbd_percentage, 
    pl.description, 
    pl.listing_date, 
    pl.retailer_id, 
    pl.product_id, 
    pl.created_by, 
    pl.updated_by, 
    pl.created_at, 
    pl.updated_at, 
    pl.is_active,
    r.store_name as retailer_name,
    r.latitude as retailer_latitude,
    r.longitude as retailer_longitude,
    r.home_page_url as retailer_home_url,
    r.menu_url as retailer_menu_url,
    p.product_name,
    (3959 * acos(
        cos(radians(?)) * cos(radians(r.latitude)) * 
        cos(radians(r.longitude) - radians(?)) + 
        sin(radians(?)) * sin(radians(r.latitude))
    )) AS distance_mile
FROM product_listings_prd pl
JOIN retailers r ON pl.retailer_id = r.retailer_id
LEFT JOIN products p ON pl.product_id = p.product_id
WHERE pl.product_id IN (sqlc.slice('product_ids'))
  AND pl.is_active = 1
  AND r.is_active = 1
  AND r.latitude IS NOT NULL
  AND r.longitude IS NOT NULL
  AND r.latitude BETWEEN ? AND ?
  AND r.longitude BETWEEN ? AND ?
  AND (3959 * acos(
       cos(radians(?)) * cos(radians(r.latitude)) * 
       cos(radians(r.longitude) - radians(?)) + 
       sin(radians(?)) * sin(radians(r.latitude))
  )) <= ?
ORDER BY distance_mile ASC, pl.price ASC;
