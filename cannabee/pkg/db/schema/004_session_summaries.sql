CREATE TABLE session_summaries (
    id INT AUTO_INCREMENT PRIMARY KEY,
    session_id CHAR(36) NOT NULL UNIQUE,
    summary TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_session_summary FOREIGN KEY (session_id) REFERENCES sessions (session_id) ON DELETE CASCADE
); 