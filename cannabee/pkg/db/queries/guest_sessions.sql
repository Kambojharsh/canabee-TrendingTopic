-- name: GetGuestSessionsByGuestID :many
SELECT session_id, guest_id, session_start_time, last_accessed_at, is_active, metadata, summary, tag, latitude, longitude
FROM guest_sessions
WHERE guest_id = ?
ORDER BY last_accessed_at DESC;

-- name: GetGuestSessionByID :one
SELECT session_id, guest_id, session_start_time, last_accessed_at, is_active, metadata, summary, tag, latitude, longitude
FROM guest_sessions
WHERE session_id = ?;

-- name: CreateGuestSession :exec
INSERT INTO guest_sessions (session_id, guest_id, session_start_time, last_accessed_at, is_active, metadata, summary, tag, latitude, longitude)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateGuestSessionAccess :exec
UPDATE guest_sessions
SET last_accessed_at = CURRENT_TIMESTAMP, is_active = ?
WHERE session_id = ?;

-- name: UpdateGuestSessionMetadata :exec
UPDATE guest_sessions
SET metadata = ?, last_accessed_at = CURRENT_TIMESTAMP
WHERE session_id = ?;

-- name: UpdateGuestSessionSummary :exec
UPDATE guest_sessions
SET summary = ?, last_accessed_at = CURRENT_TIMESTAMP
WHERE session_id = ?;

-- name: UpdateGuestSessionStatus :exec
UPDATE guest_sessions
SET is_active = ?, last_accessed_at = CURRENT_TIMESTAMP
WHERE session_id = ?;

-- name: GetLastNGuestSessionsForGuest :many
SELECT session_id, guest_id, session_start_time, last_accessed_at, is_active, metadata, summary, tag, latitude, longitude
FROM guest_sessions
WHERE guest_id = ? AND is_active = false
ORDER BY last_accessed_at DESC
LIMIT ?;

-- name: GetGuestSessionsToCleanupTags :many
SELECT session_id
FROM guest_sessions
WHERE guest_id = ? AND is_active = false
ORDER BY last_accessed_at DESC
LIMIT 18446744073709551615 OFFSET ?;

-- name: DeleteGuestSession :exec
DELETE FROM guest_sessions
WHERE session_id = ?;

-- name: UpdateGuestSessionTagAndLocation :exec
UPDATE guest_sessions
SET tag = ?, latitude = ?, longitude = ?, last_accessed_at = CURRENT_TIMESTAMP
WHERE session_id = ?;
