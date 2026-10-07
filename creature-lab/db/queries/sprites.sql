-- name: CreateCandidate :one
INSERT INTO sprite_candidates (id, species_id, form, svg, note, author, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetCandidate :one
SELECT * FROM sprite_candidates
WHERE id = $1 AND deleted_at IS NULL;

-- Every live candidate for a species, newest first within each form.
-- name: ListCandidates :many
SELECT * FROM sprite_candidates
WHERE species_id = $1 AND deleted_at IS NULL
ORDER BY form, created_at DESC;

-- name: ListAcceptedForSpecies :many
SELECT * FROM sprite_candidates
WHERE species_id = $1 AND accepted_at IS NOT NULL AND deleted_at IS NULL
ORDER BY form;

-- Every accepted sprite of every live species, for the export.
-- name: ListAllAccepted :many
SELECT sprite_candidates.* FROM sprite_candidates
JOIN species ON species.id = sprite_candidates.species_id AND species.deleted_at IS NULL
WHERE sprite_candidates.accepted_at IS NOT NULL AND sprite_candidates.deleted_at IS NULL
ORDER BY sprite_candidates.species_id, sprite_candidates.form;

-- name: ClearAccepted :exec
UPDATE sprite_candidates SET accepted_at = NULL, updated_at = NOW()
WHERE species_id = $1 AND form = $2 AND accepted_at IS NOT NULL AND deleted_at IS NULL;

-- name: MarkAccepted :one
UPDATE sprite_candidates SET accepted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteCandidate :execrows
UPDATE sprite_candidates SET deleted_at = NOW(), updated_at = NOW(), accepted_at = NULL
WHERE id = $1 AND deleted_at IS NULL;
