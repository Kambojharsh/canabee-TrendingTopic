-- Guest sessions table for chat sessions created by guest users
CREATE TABLE guest_sessions (
    session_id CHAR(36) NOT NULL PRIMARY KEY,
    guest_id CHAR(36) NOT NULL,
    session_start_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_accessed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    metadata LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL CHECK (JSON_VALID(metadata)),
    summary TEXT DEFAULT NULL,
    INDEX idx_guest_id (guest_id),
    INDEX idx_last_accessed (last_accessed_at),
    CONSTRAINT fk_guest_sessions FOREIGN KEY (guest_id) REFERENCES guest_users (guest_id) ON DELETE CASCADE,
    CONSTRAINT chk_guest_session_times CHECK (last_accessed_at >= session_start_time)
);

