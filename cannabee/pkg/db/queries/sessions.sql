-- name: GetSessionsByUserID :many
SELECT session_id, user_id, session_start_time, last_accessed_at, is_active, metadata, summary
FROM sessions
WHERE user_id = ?
ORDER BY last_accessed_at DESC;

-- name: GetSessionByID :one
SELECT session_id, user_id, session_start_time, last_accessed_at, is_active, metadata, summary
FROM sessions
WHERE session_id = ?;

-- name: CreateSession :exec
INSERT INTO sessions (session_id, user_id, session_start_time, last_accessed_at, is_active, metadata, summary)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateSessionAccess :exec
UPDATE sessions
SET last_accessed_at = CURRENT_TIMESTAMP, is_active = ?
WHERE session_id = ?;

-- name: UpdateSessionMetadata :exec
UPDATE sessions
SET metadata = ?, last_accessed_at = CURRENT_TIMESTAMP
WHERE session_id = ?;

-- name: UpdateSessionSummary :exec
UPDATE sessions
SET summary = ?, last_accessed_at = CURRENT_TIMESTAMP
WHERE session_id = ?;

-- name: UpdateSessionStatus :exec
UPDATE sessions
SET is_active = ?, last_accessed_at = CURRENT_TIMESTAMP
WHERE session_id = ?;

-- name: GetLastNSessionsForUser :many
SELECT session_id, user_id, session_start_time, last_accessed_at, is_active, metadata, summary
FROM sessions
WHERE user_id = ? AND is_active = false
ORDER BY last_accessed_at DESC
LIMIT ?;

-- name: GetSessionsToCleanupTags :many
SELECT session_id
FROM sessions
WHERE user_id = ? AND is_active = false
ORDER BY last_accessed_at DESC
LIMIT 18446744073709551615 OFFSET ?; 

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE session_id = ?;