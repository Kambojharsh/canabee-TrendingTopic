-- name: GetChatsBySessionID :many
SELECT id, session_id, message, is_user_message, created_at
FROM chats
WHERE session_id = ?
ORDER BY created_at ASC;

-- name: CreateChat :exec
INSERT INTO chats (session_id, message, is_user_message)
VALUES (?, ?, ?);

-- name: CountChatsBySessionID :one
SELECT COUNT(*) as count
FROM chats
WHERE session_id = ?; 