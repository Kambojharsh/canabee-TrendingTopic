-- name: GetRetailerByID :one
SELECT retailer_id, store_name, street, city, county, state, postal_code, country, longitude, latitude, associated_licenses, status, home_page_url, menu_url, created_by, updated_by, created_at, updated_at, is_active
FROM retailers
WHERE retailer_id = ? AND is_active = 1;

-- name: GetActiveRetailers :many
SELECT retailer_id, store_name, street, city, county, state, postal_code, country, longitude, latitude, associated_licenses, status, home_page_url, menu_url, created_by, updated_by, created_at, updated_at, is_active
FROM retailers
WHERE is_active = 1;

-- name: CreateRetailer :exec
INSERT INTO retailers (store_name, street, city, county, state, postal_code, country, longitude, latitude, associated_licenses, status, home_page_url, menu_url, created_by, updated_by, is_active)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
