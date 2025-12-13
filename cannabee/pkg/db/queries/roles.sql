-- name: GetRoles :many
SELECT role_id, name, description, created_by, created_at
FROM role
ORDER BY role_id;

-- name: GetRoleByID :one
SELECT role_id, name, description, created_by, created_at
FROM role
WHERE role_id = ?;

-- name: GetRoleByName :one
SELECT role_id, name, description, created_by, created_at
FROM role
WHERE name = ?;

-- name: CreateRole :exec
INSERT INTO role (name, description, created_by)
VALUES (?, ?, ?);

-- name: UpdateRole :exec
UPDATE role
SET name = ?, description = ?
WHERE role_id = ?;

-- name: DeleteRole :exec
DELETE FROM role
WHERE role_id = ?; 