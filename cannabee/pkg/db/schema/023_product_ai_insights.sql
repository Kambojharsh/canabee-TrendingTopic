-- Product AI Insights table stores AI-generated product information
-- This caches OpenAI responses to avoid repeated API calls for the same product
CREATE TABLE product_ai_insights (
    id BIGINT(20) UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    product_id BIGINT(20) UNSIGNED NOT NULL,
    
    -- Why this product: 2-3 lines about why and when this product is useful
    why_this_product TEXT DEFAULT NULL,
    
    -- Top feelings: JSON array of feelings (e.g., ["relaxed", "happy", "sleepy"])
    top_feelings JSON DEFAULT NULL,
    
    -- Key info: JSON object with structured product details
    -- {flavor, thc_cbd_content, strain_type, typical_time_of_use, expected_intensity, consumption_format}
    key_info JSON DEFAULT NULL,
    
    -- Ingredients and nutritional info: transparency about product contents
    ingredients TEXT DEFAULT NULL,
    
    -- Metadata
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- Unique constraint on product_id to ensure one insight per product
    UNIQUE KEY unique_product_id (product_id),
    
    -- Index for faster lookups
    KEY idx_product_id (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

