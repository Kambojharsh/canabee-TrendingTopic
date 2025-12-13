-- Guest users table for anonymous users authenticated via AWS Cognito Identity Pool
CREATE TABLE guest_users (
    guest_id CHAR(36) NOT NULL PRIMARY KEY,
    cognito_identity_id VARCHAR(255) DEFAULT NULL,
    aws_account VARCHAR(20) DEFAULT NULL,
    aws_user_id VARCHAR(255) DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_accessed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    metadata LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (JSON_VALID(metadata)),
    INDEX idx_cognito_identity (cognito_identity_id),
    INDEX idx_last_accessed (last_accessed_at)
);

