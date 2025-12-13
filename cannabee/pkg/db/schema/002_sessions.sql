CREATE TABLE sessions (
    session_id CHAR(36) NOT NULL DEFAULT (UUID()) PRIMARY KEY,
    user_id VARCHAR(45) NOT NULL,
    session_start_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_accessed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    metadata LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (JSON_VALID(metadata)),
    summary TEXT DEFAULT NULL,
    INDEX idx_user_id (user_id),
    CONSTRAINT fk_user_sessions FOREIGN KEY (user_id) REFERENCES users (user_id) ON DELETE CASCADE,
    CONSTRAINT chk_session_times CHECK (last_accessed_at >= session_start_time)
); 