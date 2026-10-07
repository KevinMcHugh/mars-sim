-- name: CreateSpecies :one
INSERT INTO species (
    id, seed, generator_rev, singular, plural, scientific_name, emoji,
    temperament, form_count, description, data, notes, created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetSpecies :one
SELECT * FROM species
WHERE id = $1 AND deleted_at IS NULL;

-- Newest first, with how many of each species' slots have an accepted sprite.
-- name: ListSpecies :many
SELECT species.*,
       (SELECT COUNT(*) FROM sprite_candidates c
         WHERE c.species_id = species.id AND c.accepted_at IS NOT NULL
           AND c.deleted_at IS NULL)::INT AS accepted_count
FROM species
WHERE deleted_at IS NULL
ORDER BY created_at DESC;

-- name: UpdateSpeciesNotes :one
UPDATE species SET notes = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteSpecies :execrows
UPDATE species SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
