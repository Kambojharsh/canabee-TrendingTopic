CREATE TABLE retailers (
    retailer_id BIGINT(20) UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    store_name TEXT,
    street TEXT,
    city TEXT,
    county TEXT,
    state TEXT,
    postal_code BIGINT(6),
    country TEXT,
    longitude DOUBLE,
    latitude DOUBLE,
    associated_licenses TEXT,
    status TEXT,
    home_page_url TEXT,
    menu_url TEXT,
    created_by VARCHAR(45),
    updated_by VARCHAR(45),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    is_active TINYINT(1) DEFAULT 1,
    
    -- Index for efficient location queries
    INDEX idx_location (latitude, longitude),
    INDEX idx_active_location (is_active, latitude, longitude)
);