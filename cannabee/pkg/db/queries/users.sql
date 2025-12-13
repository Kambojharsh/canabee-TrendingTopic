-- name: GetUserByID :one
SELECT user_id, age_group, gender, name, email, experience_level, user_type, device_id, firebase_token, device_type, password, role_id, phone_number, COALESCE(tags, '[]') as tags, COALESCE(config, '{}') as config
FROM users
WHERE user_id = ?;

-- name: GetUserByEmail :one
SELECT user_id, age_group, gender, name, email, experience_level, user_type, device_id, firebase_token, device_type, password, role_id, phone_number, COALESCE(tags, '[]') as tags, COALESCE(config, '{}') as config
FROM users
WHERE email = ?;

-- name: CreateUser :exec
INSERT INTO users (user_id, age_group, gender, name, email, experience_level, user_type, device_id, firebase_token, device_type, password, role_id, phone_number, tags, config)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateUserTags :exec
UPDATE users
SET tags = ?
WHERE user_id = ?;

-- name: GetUserTags :one
SELECT COALESCE(tags, '[]') as tags
FROM users
WHERE user_id = ?;

 