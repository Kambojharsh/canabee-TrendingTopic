CREATE TABLE products (
    product_id BIGINT(20) UNSIGNED NOT NULL AUTO_INCREMENT,
    pistil_verified TINYINT(1) DEFAULT NULL,
    taxonomy_id BIGINT(20) UNSIGNED DEFAULT NULL,
    product_name TEXT DEFAULT NULL,
    brand_id BIGINT(20) UNSIGNED DEFAULT NULL,
    product_line TEXT DEFAULT NULL,
    quantity TEXT DEFAULT NULL,
    created_by VARCHAR(45) NOT NULL DEFAULT 'SYSTEM',
    updated_by VARCHAR(45) DEFAULT 'SYSTEM',
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    is_active TINYINT(1) DEFAULT 1,
    PRIMARY KEY (product_id),
    KEY taxonomy_id (taxonomy_id),
    KEY brand_id (brand_id),
    CONSTRAINT products_ibfk_1 FOREIGN KEY (taxonomy_id) REFERENCES product_taxonomy (taxonomy_id) ON DELETE CASCADE,
    CONSTRAINT products_ibfk_2 FOREIGN KEY (brand_id) REFERENCES brand (brand_id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=latin1 COLLATE=latin1_swedish_ci;
