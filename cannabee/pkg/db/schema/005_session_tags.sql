CREATE TABLE session_tags (
    id INT AUTO_INCREMENT PRIMARY KEY,
    session_id CHAR(36) NOT NULL,
    tag VARCHAR(100) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_session_id (session_id),
    INDEX idx_tag (tag),
    CONSTRAINT fk_session_tag FOREIGN KEY (session_id) REFERENCES sessions (session_id) ON DELETE CASCADE,
    UNIQUE KEY unique_session_tag (session_id, tag)
); 